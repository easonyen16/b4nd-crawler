package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"golang.org/x/crypto/ssh/terminal"
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

func main() {
	displayWelcome()

	reader := bufio.NewReader(os.Stdin)

	// Let user input the authorization token
	fmt.Print("Enter Authorization Token (e.g., 12345|AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCdEfGh): ")
	token, _ := reader.ReadString('\n')
	token = strings.TrimSpace(token)

	// Allow user to select the artist using promptui
	prompt := promptui.Select{
		Label: "Select the artist",
		Items: []string{"鈴木絢音 (ID 37)", "菅井友香 (ID 43)"},
	}
	_, result, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return
	}

	// Extract the ID from the selection
	artistID := strings.Split(result, " (ID ")[1]
	artistID = strings.TrimRight(artistID, ")")

	url := "https://admin.b4nd.me/api/message/getChatsHistory/" + artistID
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	// Set headers
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Authorization", "Bearer "+token)
	req.Header.Add("App-Version", "1.1.12")
	req.Header.Add("Accept-Encoding", "br;q=1.0, gzip;q=0.9, deflate;q=0.8")
	req.Header.Add("Platform", "IOS")
	req.Header.Add("App-Name", "b4nd-user")
	req.Header.Add("Accept-Language", "zh-Hans-JP;q=1.0, ja-JP;q=0.9, zh-Hant-JP;q=0.8")
	req.Header.Add("User-Agent", "B4ND/1.1.12 (com.tokyo-tsushin.b4nd.prd; build:27; iOS 17.3.1) Alamofire/5.6.1")

	response, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer response.Body.Close()

	var apiResponse ApiResponse
	if err := json.NewDecoder(response.Body).Decode(&apiResponse); err != nil {
		panic(err)
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
				downloadFile(item.Upload.Path, filePath)
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
	fmt.Println(centeredLine("B4ND Crawler Version 1.0"))
	fmt.Println(centeredLine("Made by E.Y. Studio"))
	fmt.Println(emptyLine)
	fmt.Println(bottomBorder)
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

func downloadFile(url, filePath string) {
	resp, err := http.Get(url)
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
