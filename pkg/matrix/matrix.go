package matrix

import (
	"errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/justinbarrick/go-matrix/pkg/client"
	"github.com/justinbarrick/go-matrix/pkg/client/room_participation"
	"github.com/justinbarrick/go-matrix/pkg/client/session_management"
	"github.com/justinbarrick/go-matrix/pkg/client/end_to_end_encryption"
	"github.com/justinbarrick/go-matrix/pkg/models"

	"github.com/tent/canonical-json-go"
	"encoding/json"
	"os"
	"fmt"
	libolm "github.com/justinbarrick/libolm-go"
)

type Bot struct {
	accessToken string
	username    string
	password    string
	client      *client.MatrixClientServer
}

type BotState struct {
	OlmAccount string
}

type IdentityKeys struct {
	Curve25519 string `json:"curve25519"`
	Ed25519 string `json:"ed25519"`
}

type OneTimeKeys struct {
	Curve25519 map[string]string `json:"curve25519"`
}

type Olm struct {
	account libolm.Account
}

func NewOlm() (*Olm, error) {
	olm := &Olm{}

	if _, err := os.Stat("state.json"); os.IsNotExist(err) {
		olm.account = libolm.CreateNewAccount()
		olm.account.GenerateOneTimeKeys(100)

		f, err := os.Create("state.json")
		if err != nil {
			return nil, err
		}

		err = json.NewEncoder(f).Encode(BotState{
			OlmAccount: olm.account.Pickle("lol"),
		})
		if err != nil {
			return nil, err
		}

		f.Close()
	} else {
		f, err := os.Open("state.json")
		if err != nil {
			return nil, err
		}

		botState := BotState{}

		err = json.NewDecoder(f).Decode(&botState)
		if err != nil {
			return nil, err
		}

		f.Close()

		olm.account = libolm.AccountFromPickle("lol", botState.OlmAccount)
	}

	return olm, nil
}

func (o *Olm) GetIdentityKeys() IdentityKeys {
	identityKeys := IdentityKeys{}
	json.Unmarshal([]byte(o.account.GetIdentityKeys()), &identityKeys)
	return identityKeys
}

func (o *Olm) GetOneTimeKeys() OneTimeKeys {
	oneTimeKeysDecoded := OneTimeKeys{}
	json.Unmarshal([]byte(o.account.GetOneTimeKeys()), &oneTimeKeysDecoded)
	return oneTimeKeysDecoded
}

func (o *Olm) SignObj(obj interface{}) (string, error) {
	output, err := cjson.Marshal(obj)
	if err != nil {
		return "", err
	}

	signature := o.account.Sign(string(output))
	return signature, nil
}

func (o *Olm) UploadKeysParams(deviceId, userId string) (*end_to_end_encryption.UploadKeysParams, error) {
	oneTimeKeys := map[string]string{}
	for id, curve25519Key := range o.GetOneTimeKeys().Curve25519 {
		oneTimeKeys[fmt.Sprintf("curve25519:%s", id)] = curve25519Key
	}

	identityKeys := o.GetIdentityKeys()

	deviceKeys := &models.UploadKeysParamsBodyDeviceKeys{
		models.UploadKeysParamsBodyDeviceKeysAllOf0{
			Algorithms: []string{
				"m.olm.curve25519-aes-sha256",
				"m.megolm.v1.aes-sha",
			},
			DeviceID: &deviceId,
			Keys: map[string]string{
				fmt.Sprintf("curve25519:%s", deviceId): identityKeys.Curve25519,
				fmt.Sprintf("ed25519:%s", deviceId): identityKeys.Ed25519,
			},
			UserID: &userId,
		},
	}

	signature, err := o.SignObj(deviceKeys)
	if err != nil {
		return nil ,err
	}

	deviceKeys.Signatures = map[string]map[string]string{
		userId: map[string]string{
			fmt.Sprintf("ed25519:%s", deviceId): signature,
		},
	}

	uploadKeys := end_to_end_encryption.NewUploadKeysParams()
	uploadKeys.SetKeys(&models.UploadKeysParamsBody{
		DeviceKeys: deviceKeys,
		OneTimeKeys: oneTimeKeys,
	})

	return uploadKeys, nil
}

func (b *Bot) GetDeviceId() (string, error) {
	devices, err := b.client.DeviceManagement.GetDevices(nil, b)
	if err != nil {
		return "", err
	}

	deviceId := ""

	for _, device := range devices.Payload.Devices {
		deviceId = *device.DeviceID
	}

	return deviceId, nil
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

	olm, err := NewOlm()
	if err != nil {
		return bot, err
	}

	deviceId, err := bot.GetDeviceId()
	if err != nil {
		return bot, err
	}

	userId := "@musicboat:priv8.chat"

	uploadKeys, err := olm.UploadKeysParams(deviceId, userId)
	if err != nil {
		return bot, err
	}

	_, err = bot.client.EndToEndEncryption.UploadKeys(uploadKeys, &bot)
	if err != nil {
		return bot, err
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
