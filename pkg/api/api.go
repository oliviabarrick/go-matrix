package api

import (
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/trace"
	"go.opencensus.io/stats/view"
	"os"
	"time"

	"fmt"
	"github.com/gorilla/handlers"
	"github.com/justinbarrick/go-matrix/pkg/matrix"
	"github.com/justinbarrick/go-matrix/pkg/slack2matrix"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
)

func Api(bot matrix.Bot, defaultChannel, certPath, keyPath string) {
	exporter, err := prometheus.NewExporter(prometheus.Options{})
	if err != nil {
		log.Fatal(err)
	}
	view.RegisterExporter(exporter)
	view.SetReportingPeriod(1 * time.Second)
	http.Handle("/metrics", exporter)

	if os.Getenv("JAEGER_ENDPOINT") != "" {
		exporter, err := jaeger.NewExporter(jaeger.Options{
			CollectorEndpoint: os.Getenv("JAEGER_ENDPOINT"),
			Process: jaeger.Process{
				ServiceName: "slack2matrix",
			},
		})
		if err != nil {
			log.Fatal(err)
		}

		trace.RegisterExporter(exporter)
	}

	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, span := trace.StartSpan(r.Context(), "slack2matrix")
		defer span.End()

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
		span.AddAttributes(trace.StringAttribute("channel", channel))

		webhookBody, err := message.ToHTML()
		if err != nil {
			log.Println("Error marshalling message to HTML:", err.Error())
			http.Error(w, err.Error(), 500)
			return
		}

		err = bot.SendEncrypted(r.Context(), channel, webhookBody)
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
		http.ListenAndServeTLS(":8443", certPath, keyPath, &ochttp.Handler{
			Handler: handlers.LoggingHandler(os.Stderr, http.DefaultServeMux),
		})
	} else {
		log.Println("Starting slack2matrix server on :8000.")
		http.ListenAndServe(":8000", &ochttp.Handler{
			Handler: handlers.LoggingHandler(os.Stderr, http.DefaultServeMux),
		})
	}
}
