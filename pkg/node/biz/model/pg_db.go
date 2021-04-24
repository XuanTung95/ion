package biz

import (
	log "github.com/pion/ion-log"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DSN = "host=103.130.213.160 user=postgres password=dantepro195a@A dbname=postgres sslmode=disable"

var db *gorm.DB

type Users struct {
	gorm.Model
	Uuid     string
	Birth    int
	Nickname string
	Gender   int
}

func GetDb() *gorm.DB {
	if db != nil {
		return db
	}
	newdbc, err := gorm.Open(postgres.New(postgres.Config{
		DSN: DSN,
		PreferSimpleProtocol: true, // disables implicit prepared statement usage
	}), &gorm.Config{})
	if err != nil {
		log.Errorf("Cannot connect PG database: " + DSN)
		return nil
	}
	db = newdbc
	return db
}

func InitDbSchema(db *gorm.DB) {
	// Migrate the schema
	_ = db.AutoMigrate(&Users{})

	// Create
	db.Create(&Users{Uuid: "D42", Birth: 100})
	db.Create(&Users{Uuid: "D43", Birth: 112})

	// Read
	var user Users
	db.First(&user, 1) // find product with integer primary key
	db.First(&user, "uuid = ?", "D42") // find product with code D42

	// Update - update product's price to 200
	db.Model(&user).Update("Nickname", "Test Nickname 🤒 🤑 🤠")
	// Update - update multiple fields
	db.Model(&user).Updates(Users{Gender: 800, Birth: 90}) // non-zero fields
	db.Model(&user).Updates(map[string]interface{}{"Price": 200, "Code": "F42"})

	// Delete - delete product
	// db.Delete(&user)
}