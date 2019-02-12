package main

import (
	"io/ioutil"
	"flag"
//	"github.com/davecgh/go-spew/spew"
	"github.com/justinbarrick/go-matrix/pkg/matrix"
	"github.com/justinbarrick/go-matrix/pkg/slack2matrix"
	"log"
	"os"
	"github.com/gorilla/handlers"
	"net/http"
	"fmt"
)

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
		body, _ := ioutil.ReadAll(r.Body)
		log.Println("Raw request body:", string(body))

		message, err := slack2matrix.ParseSlackWebhook(body)
		if err != nil {
			log.Println("Error unmarshalling message:", err.Error())
			http.Error(w, err.Error(), 400)
			return
		}

		channel := *defaultChan
		if message.Channel != "" {
			channel = message.Channel
		}

		webhookBody, err := message.ToHTML()
		if err != nil {
			log.Println("Error marshalling message to HTML:", err.Error())
			http.Error(w, err.Error(), 500)
			return
		}

		err = bot.SendEncrypted(channel, webhookBody)
		if err != nil {
			log.Println("Error sending message:", err.Error())
			http.Error(w, err.Error(), 500)
			return
		}
		log.Printf("Sent message to '%s': %s.", channel, webhookBody)

		fmt.Fprintf(w, "Welcome to my website!")
		return
	})

	http.ListenAndServe(":8000", handlers.LoggingHandler(os.Stderr, http.DefaultServeMux))
}
