package matrix

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/justinbarrick/go-matrix/pkg/client"
	"github.com/justinbarrick/go-matrix/pkg/client/room_participation"
	"github.com/justinbarrick/go-matrix/pkg/client/session_management"
	"github.com/justinbarrick/go-matrix/pkg/client/end_to_end_encryption"
	"github.com/justinbarrick/go-matrix/pkg/client/send_to_device_messaging"
	"github.com/justinbarrick/go-matrix/pkg/models"

	"github.com/davecgh/go-spew/spew"
	"github.com/tent/canonical-json-go"
	"encoding/json"
	"os"
	"fmt"
	"log"
	libolm "github.com/justinbarrick/libolm-go"
)


type Sessions struct {
	Session libolm.Session
	UserId string
	DeviceId string
	DeviceKey string
	Ed25519Key string
}

type Bot struct {
	accessToken string
	username    string
	password    string
	olm         *Olm
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
		err = olm.Serialize()
		if err != nil {
			return olm, nil
		}
	} else {
		err = olm.Deserialize()
		if err != nil {
			return olm, nil
		}
	}

	return olm, nil
}

func (o *Olm) Deserialize() error {
	f, err := os.Open("state.json")
	if err != nil {
		return err
	}

	defer f.Close()

	botState := BotState{}

	err = json.NewDecoder(f).Decode(&botState)
	if err != nil {
		return err
	}

	o.account = libolm.AccountFromPickle("lol", botState.OlmAccount)
	return nil
}

func (o *Olm) Serialize() error {
	f, err := os.Create("state.json")
	if err != nil {
		return err
	}

	defer f.Close()

	err = json.NewEncoder(f).Encode(BotState{
		OlmAccount: o.account.Pickle("lol"),
	})
	if err != nil {
		return err
	}

	return nil
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
				"m.megolm.v1.aes-sha",
				"m.olm.curve25519-aes-sha256",
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

func (o *Olm) MarkPublished() (error) {
	o.account.MarkKeysAsPublished()
	return o.Serialize()
}

func (o *Olm) EncryptedEvent(session libolm.Encrypter, deviceId string, event interface{}) (map[string]string, error) {
	contentEncoded, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("Error encoding content: %s", err)
	}

	_, ciphertext := session.Encrypt(string(contentEncoded))

	return map[string]string{
		"algorithm": "m.megolm.v1.aes-sha2",
		"sender_key": o.GetIdentityKeys().Curve25519,
		"device_id": deviceId,
		"session_id": session.GetSessionID(),
		"ciphertext": ciphertext,
	}, nil
}

type DirectEvent struct {
	Algorithm string `json:"algorithm"`
	SenderKey string `json:"sender_key"`
	SessionId string `json:"session_id,omitempty"`
	DeviceId  string `json:"device_id,omitempty"`
	Ciphertext map[string]map[string]interface{} `json:"ciphertext"`
}

func (o *Olm) EncryptedDirectEvent(session Sessions, deviceId string, event interface{}) (*DirectEvent, error) {
	contentEncoded, err := json.Marshal(map[string]interface{}{
		"content": event,
		"type": "m.room_key",
		"sender": "@cbot:priv8.chat",
		"recipient": "@j:priv8.chat",
		"keys": map[string]string{
			"ed25519": o.GetIdentityKeys().Ed25519,
		},
		"recipient_keys": map[string]string{
			"ed25519": session.Ed25519Key,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Error encoding content: %s", err)
	}

	_, ciphertext := session.Session.Encrypt(string(contentEncoded))

	return &DirectEvent{
		Algorithm: "m.olm.v1.curve25519-aes-sha2",
		SenderKey: o.GetIdentityKeys().Curve25519,
		Ciphertext: map[string]map[string]interface{}{
			session.DeviceKey: map[string]interface{}{
				"body": ciphertext,
				"type": 0,
			},
		},
	}, nil
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

	var err error

	bot.olm, err = NewOlm()
	if err != nil {
		return bot, err
	}

	deviceId, err := bot.GetDeviceId()
	if err != nil {
		return bot, fmt.Errorf("Could not get device id: %s", err)
	}

	userId := "@cbot:priv8.chat"

	uploadKeys, err := bot.olm.UploadKeysParams(deviceId, userId)
	if err != nil {
		return bot, fmt.Errorf("Could not get upload keys parameters: %s", err)
	}

	_, err = bot.client.EndToEndEncryption.UploadKeys(uploadKeys, &bot)
	if err != nil {
		return bot, fmt.Errorf("Could not upload keys: %s", err)
	}

	err = bot.olm.MarkPublished()
	if err != nil {
		return bot, fmt.Errorf("Error publishing: %s", err)
	}

	destId := "@j:priv8.chat"
	queryParams := end_to_end_encryption.NewQueryKeysParams()
	queryParams.SetQuery(&models.QueryKeysParamsBody{
		DeviceKeys: map[string][]string{
			destId: []string{},
		},
	})

	query, err := bot.client.EndToEndEncryption.QueryKeys(queryParams, &bot)
	if err != nil {
		return bot, fmt.Errorf("Error fetching keys: %s", err)
	}

	deviceKeys := query.Payload.DeviceKeys
	wantedKeys := map[string]map[string]string{}

	for destDeviceId, _ := range deviceKeys[destId] {
		if wantedKeys[destId] == nil {
			wantedKeys[destId] = map[string]string{}
		}

		wantedKeys[destId][destDeviceId] = "signed_curve25519"
	}

	claimParams := end_to_end_encryption.NewClaimKeysParams()
	claimParams.SetQuery(&models.ClaimKeysParamsBody{
		OneTimeKeys: wantedKeys,
	})

	claim, err := bot.client.EndToEndEncryption.ClaimKeys(claimParams, &bot)
	if err != nil {
		return bot, fmt.Errorf("Error claiming keys: %s", err)
	}

	groupSession := libolm.CreateOutboundGroupSession()

	sessions := []Sessions{}

	for destId, destDevices := range claim.Payload.OneTimeKeys {
		for destDeviceId, keys := range destDevices {
			for _, keyData := range keys {
				destKey := deviceKeys[destId][destDeviceId].Keys[fmt.Sprintf("curve25519:%s", destDeviceId)]
				ed25519Key := deviceKeys[destId][destDeviceId].Keys[fmt.Sprintf("ed25519:%s", destDeviceId)]
				destOneTimeKey := keyData.Key

				session := libolm.CreateOutboundSession(bot.olm.account, destKey, destOneTimeKey)

				sessions = append(sessions, Sessions{
					Session: session,
					UserId: destId,
					DeviceId: destDeviceId,
					DeviceKey: destKey,
					Ed25519Key: ed25519Key,
				})
			}
		}
	}

	log.Println("SendToDeviceEncrypted()", groupSession.GetSessionID())

	err = bot.SendToDeviceEncrypted(sessions, map[string]interface{}{
		"algorithm": "m.megolm.v1.aes-sha2",
		"room_id": "!uPnoBWbuAnJfszrxDG:priv8.chat",
		"session_id": groupSession.GetSessionID(),
		"session_key": groupSession.GetSessionKey(),
	})
	if err != nil {
		return bot, err
	}

	log.Println("EncryptedSend()")

	bot.EncryptedSend(groupSession, "!uPnoBWbuAnJfszrxDG:priv8.chat", map[string]interface{}{
		"type": "m.room.message",
		"content": map[string]string{
			"body": "hi!",
			"msgtype": "m.text",
		},
		"room_id": "!uPnoBWbuAnJfszrxDG:priv8.chat",
	})
	if err != nil {
		return bot, err
	}

	log.Println("Sent message to !uPnoBWbuAnJfszrxDG:priv8.chat")
	return bot, nil
}

func (b *Bot) AuthenticateRequest(request runtime.ClientRequest, registry strfmt.Registry) error {
	if b.accessToken == "" {
		return fmt.Errorf("No access token set, please login.")
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
		return fmt.Errorf("Could not login: %s", err)
	}

	b.accessToken = loginOk.Payload.AccessToken
	return nil
}

func (b *Bot) SendToDeviceEncrypted(sessions []Sessions, event interface{}) error {
	deviceId, err := b.GetDeviceId()
	if err != nil {
		return fmt.Errorf("Could not get device id: %s", err)
	}

	params := send_to_device_messaging.NewSendToDeviceParams()
	params.SetEventType("m.room.encrypted")

	messages := map[string]map[string]interface{}{}

	for _, session := range sessions {
		encrypted, err := b.olm.EncryptedDirectEvent(session, deviceId, event)
		if err != nil {
			return fmt.Errorf("Could not encrypt event: %s", err)
		}
		if messages[session.UserId] == nil {
			messages[session.UserId] = map[string]interface{}{}
		}

		messages[session.UserId][session.DeviceId] = encrypted
	}

	params.SetBody(&models.SendToDeviceParamsBody{
		Messages: messages,
	})

	txid, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("Could not generate uuid: %s", err)
	}

	params.SetTxnID(txid.String())

	sentOk, err := b.client.SendToDeviceMessaging.SendToDevice(params, b)
	if err != nil {
		return fmt.Errorf("Could not send message: %s", err)
	}

	spew.Dump(sentOk)

	return nil
}


func (b *Bot) EncryptedSend(session libolm.Encrypter, channel string, message interface{}) error {
	deviceId, err := b.GetDeviceId()
	if err != nil {
		return fmt.Errorf("Could not get device id: %s", err)
	}

	encrypted, err := b.olm.EncryptedEvent(session, deviceId, message)
	if err != nil {
		return fmt.Errorf("Could not encrypt event: %s", err)
	}

	params := room_participation.NewSendMessageParams()
	params.SetRoomID(channel)
	params.SetBody(encrypted)
	params.SetEventType("m.room.encrypted")

	txid, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("Could not generate uuid: %s", err)
	}

	params.SetTxnID(txid.String())

	sentOk, err := b.client.RoomParticipation.SendMessage(params, b)
	if err != nil {
		return fmt.Errorf("Could not send message: %s", err)
	}

	spew.Dump(sentOk)

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
		return fmt.Errorf("Could not generate uuid: %s", err)
	}

	params.SetTxnID(txid.String())

	_, err = b.client.RoomParticipation.SendMessage(params, b)
	if err != nil {
		return fmt.Errorf("Could not send message: %s", err)
	}

	return nil
}
