package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	db "stories/database"
	"stories/handlers/auth"
	"stories/handlers_utils"
	"stories/pkg/utils"
	input2 "stories/pkg/workflow/input"
)

type ActionBody struct {
	Action string `json:"action"`
}

var query db.IQuery

type MapAction map[string]string

type Response struct {
	From    string `json:"from"`
	Message string `json:"message"`
}

type ArrayResponse struct {
	Values []interface{} `json:"values"`
}

func executeAction(ctx context.Context, input string) (string, error) {
	order := []byte(input)
	data, err := input2.ProcessInput(ctx, order, &query)
	if err != nil {
		return data, err
	}
	return data, nil
}

func getConnection() (db.IQuery, error) {
	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASS")
	db_name := os.Getenv("DB_NAME")
	db_port := 5432
	log.Println("connecting to host", host, "user", user, "name", db_name)
	config := db.DatabaseConfig{
		Host: host, Name: db_name, Port: db_port, User: user,
		Password: pass,
	}
	var pgQuery db.PostgresQuery
	pgQuery.Initialize(&config)
	err := pgQuery.Connect()
	if err != nil {
		return query, err
	}
	query = pgQuery.NewQuery()
	return query, nil
}

func main() {
	// Create an Echo instance
	e := echo.New()
	var err error
	query, err = getConnection()
	if err != nil {
		log.Println("error get db connection err:", err)
		return
	}

	e.Static("/data", "data")
	log.Println("db connected successfully!")
	// Define a route
	e.GET("/", serveSpecificHTML("page1.html"))

	e.POST("/change_pass", func(c echo.Context) error {
		log.Println("/change_pass change_entering POST login")
		var err error
		if query == nil {
			query, err = getConnection()
			if err != nil {
				log.Println("error get db connection err:", err)
				return c.JSON(http.StatusInternalServerError, Response{From: "stories", Message: "internal error"})
			}
		}
		handler := auth.ChangePasswordHandler{Query: query}
		handler.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	e.POST("/send_pass", func(c echo.Context) error {
		log.Println("/send_pass login_entering POST login")
		var err error
		if query == nil {
			query, err = getConnection()
			if err != nil {
				log.Println("error get db connection err:", err)
				return c.JSON(http.StatusInternalServerError, Response{From: "stories", Message: "internal error"})
			}
		}
		handler := auth.SendChangePwdHandler{Query: query}
		handler.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	// Resend email
	e.GET("/resend", func(c echo.Context) error {
		var err error
		log.Println("GET entering resend resources")
		if query == nil {
			query, err = getConnection()
			if err != nil {
				log.Println("error get db connection err:", err)
				return c.JSON(http.StatusInternalServerError, Response{From: "stories", Message: "internal error"})
			}
		}
		log.Println("database connection established")
		handler := auth.ResendCodeHandler{Query: query}
		handler.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	// confirm email via code
	e.GET("/confirm", func(c echo.Context) error {
		log.Println("entering GET confirm")
		code := c.QueryParams().Get("code")
		fmt.Println("code", code)
		othercode := c.Request().URL.Query().Get("code")
		fmt.Println("othercode", othercode)
		handler := auth.VerifyEmailHandler{Query: query}
		handler.ServeHTTP(c.Response(), c.Request())
		return nil
	})
	e.POST("/login", func(c echo.Context) error {
		log.Println("....entering POST login")
		handler := auth.UserLoginHandler{Query: query}
		handler.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.POST("/signup", func(c echo.Context) error {
		log.Println("entering POST signup")
		handler := auth.SignupUserHandler{Query: query}
		handler.ServeHTTP(c.Response(), c.Request())
		return nil
	})

	e.POST("/new", func(c echo.Context) error {
		var action ActionBody
		log.Println("entering POST new action")
		if query == nil {
			query, err = getConnection()
			if err != nil {
				log.Println("error get db connection err:", err)
				return c.JSON(http.StatusInternalServerError, Response{From: "stories", Message: "internal error"})
			}
		}
		authorizer := handlers_utils.RequestAuthorizer{Query: query}
		authorizer.ServeHTTP(c.Response(), c.Request())
		if c.Response() != nil && c.Response().Size > 0 {
			return nil
		}
		buf := new(bytes.Buffer)
		ip := c.RealIP()
		log.Println("ip", ip)
		nBytes, err := buf.ReadFrom(c.Request().Body)
		log.Println("Read ", nBytes, "nBytes", "err", err)
		if err != nil {
			return err
		}
		data := buf.String()
		log.Println("buf String data", data)
		err = json.Unmarshal([]byte(data), &action)
		if err != nil {
			log.Println("json Unmarshal err", err.Error())
			return err
		}
		log.Println("action.Action |", action.Action, "|")
		formatedAction := utils.FormatYaml(action.Action)
		log.Println("action.Action (formatted) |", formatedAction, "|")
		data, err = executeAction(authorizer.Ctx, formatedAction)
		if err != nil {
			log.Println("executeAction error", err.Error())
			return c.JSON(http.StatusBadRequest, Response{From: "input", Message: err.Error()})
		}
		if len(data) > 0 {
			dataJSON := make(map[string]interface{})
			byteData := []byte(data)
			err = json.Unmarshal(byteData, &dataJSON)
			if err != nil {
				arrayJSON := make([]interface{}, 0)
				err = json.Unmarshal(byteData, &arrayJSON)
				if err != nil {
					log.Println("unmarshall err", err.Error())
					return c.JSON(http.StatusBadRequest, Response{From: "input", Message: err.Error()})
				}
				arrayResponse := ArrayResponse{Values: arrayJSON}
				return c.JSON(http.StatusOK, arrayResponse)
			}
			return c.JSON(http.StatusOK, dataJSON)
		}

		return c.JSON(http.StatusOK, Response{From: "echo", Message: "action executed succesfully"})
	})

	// Start the server
	e.Start(":8080")
}

// readHTMLFile reads an HTML file from the assets folder
func readHTMLFile(filename string) (string, error) {
	// Construct the file path
	filePath := filepath.Join("assets", filename)

	// Read the file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read HTML file: %w", err)
	}

	return string(content), nil
}

// serveHTMLFile serves an HTML file from assets folder
func serveHTMLFile(c echo.Context) error {
	// Get filename from URL parameter or use default
	filename := c.Param("filename")
	if filename == "" {
		filename = "index.html" // default file
	}

	// Ensure the filename has .html extension
	if filepath.Ext(filename) != ".html" {
		filename += ".html"
	}

	// Read the HTML file
	content, err := readHTMLFile(filename)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "HTML file not found",
			"file":  filename,
		})
	}

	// Serve the HTML content
	return c.HTML(http.StatusOK, content)
}

// Alternative function to serve a specific HTML file
func serveSpecificHTML(htmlFile string) echo.HandlerFunc {
	return func(c echo.Context) error {
		content, err := readHTMLFile(htmlFile)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to load HTML file",
				"file":  htmlFile,
			})
		}
		return c.HTML(http.StatusOK, content)
	}
}
