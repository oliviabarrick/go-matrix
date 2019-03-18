package api

import (
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/justinbarrick/go-matrix/pkg/matrix"
	"github.com/justinbarrick/go-matrix/pkg/slack2matrix"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

func Api(bot matrix.Bot, defaultChannel, certPath, keyPath string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := ioutil.ReadAll(r.Body)
		log.Println("Raw request body:", string(body))

		message, err := slack2matrix.ParseSlackWebhook(body)
		if err != nil {
			log.Println("Error unmarshalling message:", err.Error())
			http.Error(w, err.Error(), 400)
			return
		}

		channel := defaultChannel
		if message.Channel != "" {
			channel = message.Channel
		} else if r.URL.Query().Get("channel") != "" {
			channel = r.URL.Query().Get("channel")
		}

		if channel == "" {
			log.Println("Channel not provided.")
			http.Error(w, fmt.Sprintf("Channel not provided"), 500)
			return
		}

		channel = fmt.Sprintf("!%s", strings.TrimLeft(channel, "#!"))

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

	if certPath != "" && keyPath != "" {
		log.Println("Starting slack2matrix with HTTPS on :8443.")
		http.ListenAndServeTLS(":8443", certPath, keyPath, handlers.LoggingHandler(os.Stderr, http.DefaultServeMux))
	} else {
		log.Println("Starting slack2matrix server on :8000.")
		http.ListenAndServe(":8000", handlers.LoggingHandler(os.Stderr, http.DefaultServeMux))
	}
}
