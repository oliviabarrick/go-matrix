package main

import (
	"net/url"
	"io/ioutil"
	"flag"
//	"github.com/davecgh/go-spew/spew"
	"encoding/json"
	"github.com/justinbarrick/go-matrix/pkg/matrix"
	"log"
	"os"
	"net/http"
	"fmt"
)

// Represents a slack message sent to the API
type SlackMessage struct {
	Channel     string            `json:"channel"`
	IconEmoji   string            `json:"icon_emoji"`
	Username    string            `json:"username"`
	Attachments []SlackAttachment `json:"attachments"`
}

// Represents a section of a slack message that is sent to the API
type SlackAttachment struct {
	Color     string `json:"color"`
	Title     string `json:"title"`
	TitleLink string `json:"title_link"`
	Text      string `json:"text"`
}

func main() {
	user := flag.String("user", os.Getenv("MATRIX_USER"), "Bot username.")
	pass := flag.String("pass", os.Getenv("MATRIX_PASS"), "Bot password.")
	host := flag.String("host", os.Getenv("MATRIX_HOST"), "Bot hostname.")
	defaultChan := flag.String("chan", os.Getenv("MATRIX_CHAN"), "Bot chan.")
	accessToken := flag.String("token", os.Getenv("MATRIX_TOKEN"), "Bot token.")

	flag.Parse()

	if ((*user == "" || *pass == "") && *accessToken == "") || *host == "" {
		flag.Usage()
		os.Exit(1)
	}

	bot, err := matrix.NewBot(*user, *pass, *accessToken, *host)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {
		message := SlackMessage{}

		fmt.Println(r.Header.Get("Content-Encoding"))

		if true {
			b, _ := ioutil.ReadAll(r.Body)

			values, err := url.ParseQuery(string(b))
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}

			err = json.Unmarshal([]byte(values.Get("payload")), &message)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
		} else {
			err := json.NewDecoder(r.Body).Decode(&message)
			if err != nil {
				log.Println(err)
				http.Error(w, err.Error(), 400)
				return
			}
		}

		channel := *defaultChan
		if message.Channel != "" {
			channel = message.Channel
		}

		for _, attachment := range message.Attachments {
			err = bot.SendEncrypted(channel, attachment.Text)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			log.Printf("Sent message to '%s': %s.", channel, attachment.Text)
		}

		fmt.Fprintf(w, "Welcome to my website!")
		return
	})

	http.ListenAndServe(":8000", nil)
}
