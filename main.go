package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	SPREADSHEET_ID := os.Getenv("SPREADSHEET_ID")
	CTFD_TOKEN := os.Getenv("CTFD_TOKEN")
	CTFD_URL := os.Getenv("CTFD_URL")

	ctx := context.Background()
	b, err := os.ReadFile("service-account.json")
	if err != nil {
		sendAlert("Error reading service-account.json", err)
	}
	conf, err := google.JWTConfigFromJSON(b, "https://www.googleapis.com/auth/spreadsheets")
	if err != nil {
		sendAlert("Error parsing client secret file to config", err)
	}
	sheetsClient := conf.Client(ctx)
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(sheetsClient))
	if err != nil {
		sendAlert("Unable to retrieve Sheets client", err)
	}

	userIDs, err := getUsers(CTFD_URL, CTFD_TOKEN)
	if err != nil {
		sendAlert("Error getting users", err)
	}

	var users [][]interface{}
	for _, userID := range userIDs {
		data, err := getUserData(CTFD_URL, CTFD_TOKEN, userID)
		if err != nil {
			sendAlert(fmt.Sprintf("Error getting user data for user %d", userID), err)
		}
		users = append(users, data)
	}

	log.Printf("Number of users: %d", len(users))

	writeRange := "STAGING!A2:J"
	// Name	Email	Phone Number	School	Dietary Requirements	Course and Year	Discord	Date of Birth	Student ID

	err = updateSheet(srv, SPREADSHEET_ID, writeRange, users)
	if err != nil {
		sendAlert("Error updating sheet", err)
	}

}

func sendAlert(message string, err error) {
	DISCORD_WEBHOOK := os.Getenv("DISCORD_WEBHOOK")
	DISCORD_ID_TO_PING := os.Getenv("DISCORD_ID_TO_PING")
	if DISCORD_WEBHOOK == "" {
		log.Println("DISCORD_WEBHOOK is not set")
		return
	}
	if DISCORD_ID_TO_PING != "" {
		message = fmt.Sprintf("<@%s> %s", DISCORD_ID_TO_PING, message)
	}

	payload := map[string]string{
		"content": fmt.Sprintf("%s\n```%v```", message, err),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("POST", DISCORD_WEBHOOK, bytes.NewBuffer(payloadBytes))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		log.Printf("Unexpected status code from webhook: %v", resp.StatusCode)
	}

	os.Exit(1)
}

// Note: returns users that are not hidden/banned
func getUsers(url string, token string) ([]int, error) {
	var allUserIDs []int
	page := 1

	for {
		req, err := http.NewRequest("GET", url+"/api/v1/users", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Token "+token)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		buf := new(bytes.Buffer)
		_, err = buf.ReadFrom(resp.Body)
		if err != nil {
			return nil, err
		}
		body := buf.String()

		var result map[string]interface{}
		err = json.Unmarshal([]byte(body), &result)
		if err != nil {
			return nil, err
		}

		users := result["data"].([]interface{})
		for _, user := range users {
			userMap := user.(map[string]interface{})
			userID := int(userMap["id"].(float64))
			allUserIDs = append(allUserIDs, userID)
		}

		meta := result["meta"].(map[string]interface{})
		pagination := meta["pagination"].(map[string]interface{})
		if pagination["next"] == nil {
			break
		}
		page++
	}

	return allUserIDs, nil
}

func getUserData(url string, token string, userID int) ([]interface{}, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/users/%d", url, userID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}
	body := buf.String()

	var result map[string]interface{}
	err = json.Unmarshal([]byte(body), &result)
	if err != nil {
		return nil, err
	}

	user := result["data"].(map[string]interface{})

	username := user["name"].(string)
	email := user["email"].(string)
	var phone, school, dietary, course, discord, dob, studentID string

	for _, field := range user["fields"].([]interface{}) {
		fieldMap := field.(map[string]interface{})
		fieldName := fieldMap["name"].(string)
		fieldValue := fieldMap["value"].(string)

		switch fieldName {
		case "Phone Number":
			phone = fieldValue
		case "School/Educational Institute":
			school = fieldValue
		case "Dietary Requirements":
			dietary = fieldValue
		case "Current Course and Year":
			course = fieldValue
		case "Discord username":
			discord = fieldValue
		case "What is your date of birth? (DD/MM/YYYY)":
			dob = fieldValue
		case "Student ID":
			studentID = fieldValue
		}
	}

	userData := []interface{}{
		username,
		email,
		phone,
		school,
		dietary,
		course,
		discord,
		dob,
		studentID,
	}

	return userData, nil
}

func updateSheet(srv *sheets.Service, spreadsheetId string, writeRange string, values [][]interface{}) error {
	valueRange := &sheets.ValueRange{
		Values: values,
	}

	_, err := srv.Spreadsheets.Values.Update(spreadsheetId, writeRange, valueRange).ValueInputOption("RAW").Do()
	if err != nil {
		return err
	}

	return nil
}
