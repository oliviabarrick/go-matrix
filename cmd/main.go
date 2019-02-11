package main

import (
	"flag"
	"github.com/justinbarrick/go-matrix/pkg/matrix"
	"log"
	"os"
)

func main() {
	user := flag.String("user", os.Getenv("MATRIX_USER"), "Bot username.")
	pass := flag.String("pass", os.Getenv("MATRIX_PASS"), "Bot password.")
	host := flag.String("host", os.Getenv("MATRIX_HOST"), "Bot hostname.")
	channel := flag.String("chan", os.Getenv("MATRIX_CHAN"), "Bot channel.")
	msg := flag.String("msg", os.Getenv("MATRIX_MSG"), "Bot message.")
	accessToken := flag.String("token", os.Getenv("MATRIX_TOKEN"), "Bot token.")

	flag.Parse()

	if ((*user == "" || *pass == "") && *accessToken == "") || *host == "" || *channel == "" || *msg == "" {
		flag.Usage()
		os.Exit(1)
	}

	_, err := matrix.NewBot(*channel, *msg, *user, *pass, *accessToken, *host)
	if err != nil {
		log.Fatal(err)
	}

/*
	err = bot.Send(*channel, *msg)
	if err != nil {
		log.Fatal(err)
	}
*/
}
