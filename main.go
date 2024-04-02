package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/jasonlvhit/gocron"
	"github.com/labstack/echo/v4"
	"gopkg.in/gomail.v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func gormConn() *gorm.DB {
	dsn := "root:@tcp(localhost:3306)/db_eksplorasi?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	return db
}

type Users struct {
	User_ID  int    `json:"user_id" gorm:"column:user_id;primaryKey;autoIncrement"`
	Username string `json:"username" gorm:"type:varchar(255)"`
	Email    string `json:"email" gorm:"type:varchar(255)"`
	Password string `json:"password" gorm:"type:varchar(255)"`
}

type Subscriptions struct {
	ID_Payment    int    `json:"id_payment" gorm:"column:id_payment;primaryKey"`
	User_ID       int    `json:"user_id" gorm:"column:user_id"`
	Layanan_ID    int    `json:"layanan_id" gorm:"column:layanan_id"`
	Jenis_Payment string `json:"jenis_payment" gorm:"type:varchar(255);column:jenis_payment"`
	Active        bool   `json:"status_subscription" gorm:"column:active"`
}

type Services struct {
	Layanan_ID       int    `json:"layanan_id" gorm:"column:layanan_id;primaryKey;autoIncrement"`
	Nama_Layanan     string `json:"nama_layanan" gorm:"column:nama_layanan;type:varchar(255)"`
	Penyedia_Layanan string `json:"penyedia_layanan" gorm:"column:penyedia_layanan;type:varchar(255)"`
	Harga            int    `json:"harga" gorm:"column:harga"`
}

var ring = redis.NewClient(&redis.Options{
	Addr:     "localhost:6379",
	Password: "", // no password set
	DB:       0,  // use default DB
})

func SetRedis(rdb *redis.Client, key string, value string, expiration int) {
	err := rdb.Set(ctx, key, value, 0).Err()
	if err != nil {
		log.Fatal(err)
	}
}

func GetRedis(rdb *redis.Client, key string) string {
	val, err := rdb.Get(ctx, key).Result()

	if err != nil {
		log.Fatal(err)
	}
	return val
}

var ctx = context.Background()

type Response struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func SendMail(to string, subject string, content string) {
	email := gomail.NewMessage()
	email.SetHeader("From", "noreply@carihati.online")
	email.SetHeader("To", to)
	email.SetHeader("Subject", subject)
	email.SetBody("text/plain", content)

	sender := gomail.NewDialer("smtppro.zoho.com", 587, "noreply@carihati.online", "rw3NBFZnWeGk")

	if err := sender.DialAndSend(email); err != nil {
		panic(err)
	}
}

func GetUserData(user_id int) {
	db := gormConn()
	var user Users
	user.User_ID = user_id
	result := db.First(&user)
	if result.Error == nil {
		SetRedis(ring, "userId", strconv.Itoa(user.User_ID), 0)
		SetRedis(ring, "userEmail", user.Email, 0)
	} else {
		panic(result.Error)
	}
}

func insertUser(c echo.Context) error {
	db := gormConn()

	user := new(Users)
	user.Username = c.FormValue("username")
	user.Email = c.FormValue("email")
	user.Password = c.FormValue("password")

	query := db.Select("username", "email", "password").Create(&user)
	if query.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal memasukkan data pengguna",
		})
	}
	subscription := new(Subscriptions)
	subscription.Jenis_Payment = "OvO"
	subscription.Layanan_ID = 1
	subscription.User_ID = user.User_ID
	subscription.Active = false
	query2 := db.Select("user_id", "layanan_id", "jenis_payment", "active").Create(&subscription)
	if query2.Error != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"message": "Gagal memasukkan data subscription",
		})
	}
	go SendMail(user.Email, "Account Successfully Created!", "Welcome "+user.Username+" To The Netflix Platform, Please Enjoy The film :)")
	GetUserData(user.User_ID)
	return c.JSON(http.StatusOK, user)
}

func Subscribe(c echo.Context) error {
	db := gormConn()
	id, _ := strconv.Atoi(c.QueryParam("layanan_id"))

	user_id := GetRedis(ring, "userId")
	email := GetRedis(ring, "userEmail")
	var response Response
	if err := ring.Get(ctx, "userData"); err != nil {
		result := db.Table("subscriptions").Where("user_id=? AND layanan_id=?", user_id, id).Update("active", true)
		if result.Error == nil {
			response.Status = http.StatusOK
			response.Message = "Success Subscribe"
			SendMail(email, "Subscription Activation Success", "Congratulations your monthly subscription to Spotify was successfully activated")
		} else {
			response.Status = http.StatusInternalServerError
			response.Message = "Fail Subscribe"
		}
	}
	return c.JSON(response.Status, response)
}

func Unsubscribe(c echo.Context) error {
	db := gormConn()
	id, _ := strconv.Atoi(c.QueryParam("layanan_id"))

	user_id := GetRedis(ring, "userId")
	email := GetRedis(ring, "userEmail")
	var response Response
	if err := ring.Get(ctx, "userData"); err != nil {
		result := db.Table("subscriptions").Where("user_id=? AND layanan_id=?", user_id, id).Update("active", false)
		if result.Error == nil {
			response.Status = http.StatusOK
			response.Message = "Successful Termination"
			SendMail(email, "Subscription Terminated", "I'm sorry to see you go, Please contact us if you'd like to communicate any issues.")
		} else {
			response.Status = http.StatusInternalServerError
			response.Message = "Fail Unsubscribe"
		}
	}
	return c.JSON(response.Status, response)
}

func CheckActive() bool {
	db := gormConn()
	user_id := GetRedis(ring, "userId")
	var subscription Subscriptions
	if user_id != "" {
		db.Where("user_id=?", user_id).First(&subscription)
	}
	return subscription.Active
}

func task() {
	active := CheckActive()
	if !active {
		SendMail(GetRedis(ring, "userEmail"), "Activate your Subscription", "Activate full Spotify Premium to enjoy all the features")
	}
}

func main() {
	router := echo.New()
	go GetUserData(1)
	time.Sleep(2 * time.Second)
	gocron.Start()
	gocron.Every(20).Seconds().Do(task)
	router.PUT("/subscribe", Subscribe)
	router.POST("/users", insertUser)
	router.PUT("/unsubscribe", Unsubscribe)
	router.Logger.Fatal(router.Start(":1323"))
}
