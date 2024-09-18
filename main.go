package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	proxyAddr = flag.String("proxy", "", "Proxy address in the format ip:port")
)

type ApiResponse struct {
	Success bool   `json:"success"`
	Data    []Data `json:"data"`
}

type Data struct {
	ID      int     `json:"id"`
	SendBy  int     `json:"send_by"`
	Message string  `json:"message"`
	SendAt  int64   `json:"send_at"`
	Upload  *Upload `json:"upload"`
}

type Upload struct {
	Path string `json:"path"`
}

type LoginResponse struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
}

var version string = "1.4.0"

func main() {
	displayWelcome()

	flag.Parse()

	if len(flag.Args()) > 0 {
		fmt.Printf("Error: Unsupported command-line arguments: %v\n", flag.Args())
		flag.Usage()
		os.Exit(1)
	}

	var client *http.Client
	if *proxyAddr != "" {
		proxyURL, err := url.Parse("http://" + *proxyAddr)
		if err != nil {
			fmt.Println("Error parsing proxy address:", err)
			return
		}
		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
		fmt.Println("Using proxy:", *proxyAddr)
	} else {
		client = &http.Client{}
	}

	// Allow user to select the artist using promptui
	prompt := promptui.Select{
		Label: "Select the artist",
		Items: []string{"松村沙友理 (ID 36)", "鈴木絢音 (ID 37)", "菅井友香 (ID 43)", "齊藤京子 (ID 1)"},
	}
	_, artistResult, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return
	}

	// Extract the ID from the selection
	artistID := strings.Split(artistResult, " (ID ")[1]
	artistID = strings.TrimRight(artistID, ")")

	// Set API base URL and headers based on the selected artist
	apiBaseURL := "https://admin.b4nd.me/api"
	appVersion := "1.1.20"
	appName := "b4nd-user"

	if strings.Contains(artistResult, "齊藤京子") {
		apiBaseURL = "https://api-prd.saitokyoko-message.jp/api"
		appVersion = "1.0.2"
		appName = "kyoko-user"
	}

	// Create prompt menu with options
	prompt = promptui.Select{
		Label: "Select Login Method",
		Items: []string{"Use Account Password", "Enter Token Directly"},
	}
	_, result, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return
	}

	var token string
	switch result {
	case "Use Account Password":
		token, err = loginWithCredentials(client, apiBaseURL, appVersion, appName)
		if err != nil {
			fmt.Println("Error logging in:", err)
			return
		}
	case "Enter Token Directly":
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("Enter Authorization Token (e.g., 12345|AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfGh): ")
			token, _ = reader.ReadString('\n')
			token = strings.TrimSpace(token)

			// Check if the token format is correct
			if strings.Contains(token, "|") && len(strings.Split(token, "|")) == 2 {
				parts := strings.Split(token, "|")
				if parts[0] != "" && parts[1] != "" {
					break // Token format is correct
				}
			}

			fmt.Println("Invalid token format. Please ensure the token is in the correct 'ID|Token' format.")
		}
	}

	fmt.Println("Token:", token)

	// Now construct the URL for API request
	url := apiBaseURL + "/message/getChatsHistory/" + artistID
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	// Set headers
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("App-Version", appVersion)
	req.Header.Add("Accept-Encoding", "br;q=1.0, gzip;q=0.9, deflate;q=0.8")
	req.Header.Add("Platform", "IOS")
	req.Header.Add("App-Name", appName)
	req.Header.Add("Accept-Language", "zh-Hans-JP;q=1.0, ja-JP;q=0.9, zh-Hant-JP;q=0.8")
	req.Header.Add("User-Agent", "B4ND/"+appVersion+" (com.tokyo-tsushin.b4nd.prd; build:27; iOS 17.3.1) Alamofire/5.6.1")

	response, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making the request:", err)
		return
	}
	defer response.Body.Close()

	var apiResponse ApiResponse
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		fmt.Println("Error decoding response:", err)
		fmt.Println("Token may be incorrect or expired, received unexpected response structure.")
		return
	}

	if !apiResponse.Success {
		var respData map[string]interface{}
		json.NewDecoder(response.Body).Decode(&respData)
		if message, ok := respData["message"].(string); ok {
			fmt.Printf("Token may be incorrect or expired: %s\n", message)
		} else {
			fmt.Println("Token may be incorrect or expired, received unexpected response structure.")
		}
		return
	}

	for _, item := range apiResponse.Data {
		folderPath := filepath.Join(".", strconv.Itoa(item.SendBy))
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			panic(err)
		}

		// Handle text message
		if item.Message != "" {
			fileName := strconv.Itoa(item.SendBy) + "_" + strconv.FormatInt(item.SendAt, 10) + ".txt"
			filePath := filepath.Join(folderPath, fileName)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				writeTextFile(filePath, item.Message, item.SendAt)
				fmt.Printf("[%s]: %s\n", time.Unix(item.SendAt, 0).Format("2006-01-02 15:04:05"), item.Message)
			}
		}

		// Handle upload
		if item.Upload != nil {
			filePath := filepath.Join(folderPath, filepath.Base(item.Upload.Path))
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				downloadFile(client, item.Upload.Path, filePath)
				fmt.Printf("[%s]: Downloaded file %s\n", time.Unix(item.SendAt, 0).Format("2006-01-02 15:04:05"), filepath.Base(item.Upload.Path))
				os.Chtimes(filePath, time.Now(), time.Unix(item.SendAt, 0))
			}
		}
	}
}

func displayWelcome() {
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 80
	}
	topBorder := "┌" + strings.Repeat("─", width-2) + "┐"
	bottomBorder := "└" + strings.Repeat("─", width-2) + "┘"
	emptyLine := "│" + strings.Repeat(" ", width-2) + "│"

	centeredLine := func(text string) string {
		space := (width - 2 - len(text)) / 2
		return "│" + strings.Repeat(" ", space) + text + strings.Repeat(" ", width-2-space-len(text)) + "│"
	}

	fmt.Println(topBorder)
	fmt.Println(emptyLine)
	fmt.Println(centeredLine("B4ND Crawler Version " + version))
	fmt.Println(centeredLine("Made by E.Y. Studio"))
	fmt.Println(emptyLine)
	fmt.Println(bottomBorder)
}

func loginWithCredentials(client *http.Client, apiBaseURL, appVersion, appName string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var email, password string

	for {
		fmt.Print("Enter Email: ")
		email, _ = reader.ReadString('\n')
		email = strings.TrimSpace(email)
		if email != "" {
			break
		}
		fmt.Println("Email cannot be empty. Please try again.")
	}

	for {
		fmt.Print("Enter Password: ")
		password, _ = reader.ReadString('\n')
		password = strings.TrimSpace(password)
		if password != "" {
			break
		}
		fmt.Println("Password cannot be empty. Please try again.")
	}

	data := map[string]string{
		"email":    email,
		"password": password,
	}
	body, _ := json.Marshal(data)

	loginURL := apiBaseURL + "/user/login"

	req, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}

	req.Header.Add("app-version", appVersion)
	req.Header.Add("app-name", appName)
	req.Header.Add("Platform", "IOS")
	req.Header.Add("content-type", "application/json; charset=UTF-8")
	req.Header.Add("accept-encoding", "gzip")
	req.Header.Add("User-Agent", "B4ND/"+appVersion+" (com.tokyo-tsushin.b4nd.prd; build:27; iOS 17.3.1) Alamofire/5.6.1")

	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	//fmt.Println("Response Body:", string(responseBody))

	var respData map[string]interface{}
	if err := json.Unmarshal(responseBody, &respData); err != nil {
		return "", err
	}

	if messages, ok := respData["messages"].([]interface{}); ok && len(messages) > 0 {
		if msg, ok := messages[0].(string); ok {
			fmt.Println("Login failed:", msg)
			return loginWithCredentials(client, apiBaseURL, appVersion, appName)
		}
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(responseBody, &loginResp); err != nil {
		return "", err
	}

	return loginResp.Data.Token, nil

}

func writeTextFile(filePath, content string, sendAt int64) {
	content = strings.ReplaceAll(content, "\\r\\n", "\n")
	file, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		panic(err)
	}

	os.Chtimes(filePath, time.Now(), time.Unix(sendAt, 0))
}

func downloadFile(client *http.Client, url, filePath string) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	out, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		panic(err)
	}
}
