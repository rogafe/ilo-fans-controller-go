package main

import (
	"context"
	"html/template"
	"log"
	"os"
	"strings"

	"github.com/Masterminds/sprig/v3"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"github.com/joho/godotenv"

	"ilo-fans-controller-go/internals/config"
	"ilo-fans-controller-go/internals/console"
	"ilo-fans-controller-go/internals/database"
	"ilo-fans-controller-go/internals/handlers"
	"ilo-fans-controller-go/internals/router"
	"ilo-fans-controller-go/internals/services/advancedprofiles"
	"ilo-fans-controller-go/internals/services/ilo"
	"ilo-fans-controller-go/internals/services/presets"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.SetPrefix("ilo-fans-controller-go: ")
	log.SetOutput(os.Stdout)

	// if ENV is set to dev use godotenv
	env := os.Getenv("ENV")
	env = strings.ToLower(env)
	log.Println("ENV: ", env)
	if strings.Contains(env, "dev") {
		log.Println("Loading .env file")
		err := godotenv.Load()
		if err != nil {
			log.Fatalf("Error loading .env file: %v", err)
		}
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	engine := html.New("./views", ".html")
	engine.AddFuncMap(sprig.FuncMap())
	engine.AddFunc("safeJS", func(value string) template.JS {
		return template.JS(value)
	})

	engine.Debug(true)

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal(err)
	}

	presetService := presets.New(db)
	if err := presetService.EnsureDefaults(context.Background()); err != nil {
		log.Fatal(err)
	}
	advancedProfileService := advancedprofiles.New(db)
	if err := advancedProfileService.EnsureDefaults(context.Background()); err != nil {
		log.Fatal(err)
	}

	hub := console.NewHub()
	handler := handlers.New(cfg, hub, ilo.New(cfg, hub), presetService, advancedProfileService)

	app := fiber.New(fiber.Config{
		Views:       engine,
		ViewsLayout: "layouts/base",
	})

	app.Use(logger.New())
	app.Static("/assets", "./assets/dist")
	router.Register(app, handler)

	log.Fatal(app.Listen(":" + cfg.Port))
}
