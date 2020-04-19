package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/mail"
	"os/exec"
	"strings"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/spf13/viper"
)

func main() {
	viper.SetConfigFile(".env")
	e := viper.ReadInConfig()

	if e != nil {
		panic(e)
	}
	imapServer := viper.Get("IMAP_SERVER").(string)
	userName := viper.Get("USERNAME").(string)
	passWord := viper.Get("PASSWORD").(string)
	url := viper.Get("CLOUD_AUTOML_URL").(string)
	log.Println("Connecting to server...")

	cmd := exec.Command("bash", "-c", "gcloud auth application-default print-access-token")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	bearer := out.String()
	bearer = strings.ReplaceAll(bearer, "\n", "")

	// Connect to server
	c, err := client.DialTLS(imapServer, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Connected")
	defer c.Logout()

	// Login
	if err := c.Login(userName, passWord); err != nil {
		log.Fatal(err)
	}
	log.Println("Logged in")

	// Select INBOX
	mbox, err := c.Select("INBOX", false)
	if err != nil {
		log.Fatal(err)
	}

	if mbox.Messages == 0 {
		log.Fatal("No message in mailbox")
	}
	seqSet := new(imap.SeqSet)
	seqSet.AddNum(mbox.Messages)

	section := &imap.BodySectionName{}
	items := []imap.FetchItem{section.FetchItem()}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqSet, items, messages)
	}()

	log.Println("Last message Classification:")
	msg := <-messages
	r := msg.GetBody(section)
	if r == nil {
		log.Fatal("Server didn't returned message body")
	}

	if err := <-done; err != nil {
		log.Fatal(err)
	}

	m, err := mail.ReadMessage(r)
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(m.Body)
	if err != nil {
		log.Fatal(err)
	}
	x := string(body)
	y := strings.Split(x, "Content-Type: text/html;")

	if len(y) > 0 {
		x = y[0]
	}

	var jsonStr = []byte(`{
			"payload": {
				"textSnippet": {
					"content": "` + x + `",
					"mime_type": "text/plain"
				}
			}
		}`)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		panic(err)
	}
	bearer = "Bearer " + bearer
	req.Header.Set("Authorization", bearer)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	body, _ = ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
	log.Println("Done!")
}
