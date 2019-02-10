package matrix

import (
	"errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/justinbarrick/go-matrix/pkg/client"
	"github.com/justinbarrick/go-matrix/pkg/client/room_participation"
	"github.com/justinbarrick/go-matrix/pkg/client/session_management"
	"github.com/justinbarrick/go-matrix/pkg/models"
)

type Bot struct {
	accessToken string
	username    string
	password    string
	client      *client.MatrixClientServer
}

func NewBot(username, password, accessToken, host string) (Bot, error) {
	transport := client.DefaultTransportConfig()

	bot := Bot{
		username:    username,
		password:    password,
		accessToken: accessToken,
		client:      client.NewHTTPClientWithConfig(nil, transport.WithHost(host)),
	}

	if username != "" && password != "" {
		err := bot.Login()
		if err != nil {
			return bot, err
		}
	}

	return bot, nil
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
		Type:     &loginType,
		User:     b.username,
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
		"body":    message,
	})
	params.SetEventType("m.room.message")

	txid, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	params.SetTxnID(txid.String())

	_, err = b.client.RoomParticipation.SendMessage(params, b)
	if err != nil {
		return err
	}

	return nil
}
