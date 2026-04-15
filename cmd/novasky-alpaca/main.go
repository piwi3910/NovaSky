package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/piwi3910/NovaSky/internal/db"
	"github.com/piwi3910/NovaSky/internal/models"
)

var (
	safetyConnected    bool
	observingConnected bool
	serverTxID         int
)

func alpacaResponse(value interface{}, clientTxID int, errNum int, errMsg string) fiber.Map {
	serverTxID++
	return fiber.Map{
		"Value": value, "ClientTransactionID": clientTxID,
		"ServerTransactionID": serverTxID, "ErrorNumber": errNum, "ErrorMessage": errMsg,
	}
}

func main() {
	log.Println("[alpaca] Starting...")
	db.Init()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	// Discovery
	app.Get("/management/apiversions", func(c *fiber.Ctx) error { return c.JSON([]int{1}) })
	app.Get("/management/v1/configureddevices", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"Value": []fiber.Map{
			{"DeviceName": "NovaSky SafetyMonitor", "DeviceType": "SafetyMonitor", "DeviceNumber": 0, "UniqueID": "novasky-safety-001"},
			{"DeviceName": "NovaSky ObservingConditions", "DeviceType": "ObservingConditions", "DeviceNumber": 0, "UniqueID": "novasky-observing-001"},
		}})
	})

	// SafetyMonitor
	sm := app.Group("/api/v1/safetymonitor/0")
	sm.Get("/issafe", func(c *fiber.Ctx) error {
		txID, _ := strconv.Atoi(c.Query("ClientTransactionID", "0"))
		if !safetyConnected {
			return c.JSON(alpacaResponse(false, txID, 1024, "Not connected"))
		}
		var s models.SafetyState
		db.GetDB().Order("evaluated_at DESC").First(&s)
		return c.JSON(alpacaResponse(s.State == "SAFE", txID, 0, ""))
	})
	sm.Get("/connected", func(c *fiber.Ctx) error {
		txID, _ := strconv.Atoi(c.Query("ClientTransactionID", "0"))
		return c.JSON(alpacaResponse(safetyConnected, txID, 0, ""))
	})
	sm.Put("/connected", func(c *fiber.Ctx) error {
		safetyConnected = c.FormValue("Connected") == "true" || c.FormValue("Connected") == "True"
		txID, _ := strconv.Atoi(c.FormValue("ClientTransactionID", "0"))
		return c.JSON(alpacaResponse(nil, txID, 0, ""))
	})
	sm.Get("/name", func(c *fiber.Ctx) error {
		return c.JSON(alpacaResponse("NovaSky SafetyMonitor", 0, 0, ""))
	})
	sm.Get("/description", func(c *fiber.Ctx) error {
		return c.JSON(alpacaResponse("NovaSky observatory safety", 0, 0, ""))
	})
	sm.Get("/interfaceversion", func(c *fiber.Ctx) error {
		return c.JSON(alpacaResponse(1, 0, 0, ""))
	})

	// ObservingConditions
	oc := app.Group("/api/v1/observingconditions/0")
	oc.Get("/cloudcover", func(c *fiber.Ctx) error {
		txID, _ := strconv.Atoi(c.Query("ClientTransactionID", "0"))
		if !observingConnected {
			return c.JSON(alpacaResponse(0, txID, 1024, "Not connected"))
		}
		var a models.AnalysisResult
		db.GetDB().Order("analyzed_at DESC").First(&a)
		return c.JSON(alpacaResponse(int(a.CloudCover*100), txID, 0, ""))
	})
	oc.Get("/temperature", func(c *fiber.Ctx) error {
		txID, _ := strconv.Atoi(c.Query("ClientTransactionID", "0"))
		if !observingConnected {
			return c.JSON(alpacaResponse(0, txID, 1024, "Not connected"))
		}
		var r models.SensorReading
		db.GetDB().Where("sensor_type = ?", "temperature").Order("recorded_at DESC").First(&r)
		return c.JSON(alpacaResponse(r.Value, txID, 0, ""))
	})
	oc.Get("/connected", func(c *fiber.Ctx) error {
		txID, _ := strconv.Atoi(c.Query("ClientTransactionID", "0"))
		return c.JSON(alpacaResponse(observingConnected, txID, 0, ""))
	})
	oc.Put("/connected", func(c *fiber.Ctx) error {
		observingConnected = c.FormValue("Connected") == "true" || c.FormValue("Connected") == "True"
		txID, _ := strconv.Atoi(c.FormValue("ClientTransactionID", "0"))
		return c.JSON(alpacaResponse(nil, txID, 0, ""))
	})
	oc.Get("/name", func(c *fiber.Ctx) error {
		return c.JSON(alpacaResponse("NovaSky ObservingConditions", 0, 0, ""))
	})
	oc.Get("/interfaceversion", func(c *fiber.Ctx) error {
		return c.JSON(alpacaResponse(1, 0, 0, ""))
	})

	port := os.Getenv("ALPACA_PORT")
	if port == "" {
		port = "11111"
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		app.Shutdown()
	}()

	log.Printf("[alpaca] Server started on port %s", port)
	app.Listen(":" + port)
}
