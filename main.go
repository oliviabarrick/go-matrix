package main

import (
    "errors"
    "flag"
    "log"
    "os"
    "github.com/go-openapi/runtime"
    "github.com/go-openapi/strfmt"
    "github.com/justinbarrick/go-matrix/client/session_management"
    "github.com/justinbarrick/go-matrix/client/room_participation"
    "github.com/justinbarrick/go-matrix/client"
    "github.com/justinbarrick/go-matrix/models"
)

type Bot struct {
    username      string
    password      string
    accessToken   string
    client       *client.MatrixClientServer
}

func NewBot(username string, password string, host string) (Bot, error) {
    transport := client.DefaultTransportConfig()

    bot := Bot{
        username: username,
        password: password,
        client: client.NewHTTPClientWithConfig(nil, transport.WithHost(host)),
    }

    err := bot.Login()
    return bot, err
}

func (b *Bot) AuthenticateRequest(request runtime.ClientRequest, registry strfmt.Registry) error {
    if b.accessToken == "" {
        return errors.New("No access token set, please login.")
    }

    request.SetQueryParam("access_token", b.accessToken)
    return nil
}

func (b *Bot) Login() error {
    loginType := "m.login.password"

    loginParams := session_management.NewLoginParams()
    loginParams.SetBody(&models.LoginParamsBody{
        Type: &loginType,
        User: b.username,
        Password: b.password,
    })

    loginOk, err := b.client.SessionManagement.Login(loginParams)
    if err != nil {
        return err
    }

    b.accessToken = loginOk.Payload.AccessToken
    return nil
}

func (b *Bot) Send(channel, message string) error {
    params := room_participation.NewSendMessageParams()
    params.SetRoomID(channel)
    params.SetBody(map[string]string{
        "msgtype": "m.text",
        "body": message,
    })
    params.SetEventType("m.room.message")

    _, err := b.client.RoomParticipation.SendMessage(params, b)
    if err != nil {
        return err
    }

    return nil
}

func main() {
    user := flag.String("user", os.Getenv("MATRIX_USER"), "Bot username.")
    pass := flag.String("pass", os.Getenv("MATRIX_PASS"), "Bot password.")
    host := flag.String("host", os.Getenv("MATRIX_HOST"), "Bot hostname.")
    channel := flag.String("chan", os.Getenv("MATRIX_CHAN"), "Bot channel.")
    msg := flag.String("msg", os.Getenv("MATRIX_MSG"), "Bot message.")

    flag.Parse()

    if *user == "" || *pass == "" || *host == "" || *channel == "" || *msg == "" {
        flag.Usage()
        os.Exit(1)
    }

    bot, err := NewBot(*user, *pass, *host)
    if err != nil {
        log.Fatal(err)
    }

    err = bot.Send(*channel, *msg)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("Sent message to", *channel)
}
