package matrix

import (
	"context"
	"go.opencensus.io/plugin/ochttp"
	httptransport "github.com/go-openapi/runtime/client"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/justinbarrick/go-matrix/pkg/client"
	"github.com/justinbarrick/go-matrix/pkg/client/end_to_end_encryption"
	"github.com/justinbarrick/go-matrix/pkg/client/room_membership"
	"github.com/justinbarrick/go-matrix/pkg/client/room_participation"
	"github.com/justinbarrick/go-matrix/pkg/client/send_to_device_messaging"
	"github.com/justinbarrick/go-matrix/pkg/client/session_management"
	"github.com/justinbarrick/go-matrix/pkg/client/user_data"
	"github.com/justinbarrick/go-matrix/pkg/models"
	"jaytaylor.com/html2text"

	"encoding/json"
	"fmt"
	libolm "github.com/justinbarrick/libolm-go"
	"os"
	"path/filepath"
	"strings"
)

func Serialize(b Bot, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(b)
}

func Unserialize(path string) (Bot, error) {
	b := Bot{}

	f, err := os.Open(path)
	if err != nil {
		return b, err
	}
	defer f.Close()

	if err := json.NewDecoder(f).Decode(&b); err != nil {
		return b, err
	}

	return b, b.Init()
}

// Represents an encrypted event that will be sent directly to a device.
type DirectEvent struct {
	Algorithm  string                            `json:"algorithm"`
	SenderKey  string                            `json:"sender_key"`
	SessionId  string                            `json:"session_id,omitempty"`
	DeviceId   string                            `json:"device_id,omitempty"`
	Ciphertext map[string]map[string]interface{} `json:"ciphertext"`
}

// A bot instance that can send messages to Matrix channels.
type Bot struct {
	UserId      string         `json:"userId"`
	DeviceId    string         `json:"deviceId"`
	AccessToken string         `json:"accessToken"`
	Server      string         `json:"server"`
	Olm         *libolm.Matrix `json:"olm"`
	client      *client.MatrixClientServer
	shookDevices map[string]bool
	groupSession libolm.GroupSession
	sessions []libolm.UserSession
}

// Initialize a new bot instance. Most provide either username+password or accessToken.
func NewBot(server string) (Bot, error) {
	bot := Bot{
		Server: server,
	}

	return bot, bot.Init()
}

// Initialize a bot from the configuration.
func (b *Bot) Init() (err error) {
	transport := client.DefaultTransportConfig()
	b.client = client.NewHTTPClientWithConfig(nil, transport.WithHost(b.Server))
	b.client.Transport.(*httptransport.Runtime).Transport = &ochttp.Transport{}
	b.shookDevices = map[string]bool{}
	b.sessions = []libolm.UserSession{}
	b.groupSession = libolm.CreateOutboundGroupSession()
	return nil
}

// Implement ClientAuthInfoWriter by adding the bot's access token to all API requests.
func (b *Bot) AuthenticateRequest(request runtime.ClientRequest, registry strfmt.Registry) error {
	if b.AccessToken == "" {
		return fmt.Errorf("No access token set, please login.")
	}

	request.SetQueryParam("access_token", b.AccessToken)
	return nil
}

// Login with a username and password, not needed if accessToken is provided.
func (b *Bot) Register(c context.Context, username, password string) error {
	registerParams := user_data.NewRegisterParamsWithContext(c)
	registerParams.SetBody(&models.RegisterParamsBody{
		Username: username,
		Password: password,
	})

	registerOk, err := b.client.UserData.Register(registerParams)
	if err == nil {
		b.AccessToken = registerOk.Payload.AccessToken
		b.DeviceId = registerOk.Payload.DeviceID
		b.UserId = *registerOk.Payload.UserID
		return nil
	}

	unauthorized, ok := err.(*user_data.RegisterUnauthorized)
	if !ok {
		return fmt.Errorf("Could not register: %s", err)
	}

	loginType := "m.login.dummy"

	registerParams.SetBody(&models.RegisterParamsBody{
		Username:                 username,
		Password:                 password,
		InitialDeviceDisplayName: username,
		Auth: &models.RegisterParamsBodyAuth{
			Session: unauthorized.Payload.Session,
			Type:    &loginType,
		},
	})

	registerOk, err = b.client.UserData.Register(registerParams)
	if err != nil {
		return fmt.Errorf("Could not login: %s", err)
	}

	b.AccessToken = registerOk.Payload.AccessToken
	b.DeviceId = registerOk.Payload.DeviceID
	b.UserId = *registerOk.Payload.UserID
	b.Olm = libolm.NewMatrix()

	err = b.UploadKeys(c)
	if err != nil {
		return err
	}

	return nil
}

// Login with a username and password, not needed if accessToken is provided.
func (b *Bot) Login(c context.Context, username, password string) error {
	loginType := "m.login.password"

	loginParams := session_management.NewLoginParamsWithContext(c)
	loginParams.SetBody(&models.LoginParamsBody{
		Type:     &loginType,
		User:     username,
		Password: password,
	})

	loginOk, err := b.client.SessionManagement.Login(loginParams)
	if err != nil {
		return fmt.Errorf("Could not login: %s", err)
	}

	b.AccessToken = loginOk.Payload.AccessToken
	b.DeviceId = loginOk.Payload.DeviceID
	b.UserId = loginOk.Payload.UserID
	b.Olm = libolm.NewMatrix()

	err = b.UploadKeys(c)
	if err != nil {
		return err
	}

	return nil
}

// Logout an access token.
func (b *Bot) Logout(c context.Context) error {
	logoutParams := session_management.NewLogoutParamsWithContext(c)
	_, err := b.client.SessionManagement.Logout(logoutParams, b)
	return err
}

// Logout all access tokens for the device.
func (b *Bot) LogoutAll(c context.Context) error {
	logoutParams := session_management.NewLogoutAllParamsWithContext(c)
	_, err := b.client.SessionManagement.LogoutAll(logoutParams, b)
	return err
}

// Upload our keys to the server.
func (b *Bot) UploadKeys(c context.Context) error {
	uploadKeys, err := b.Olm.UploadKeysParams(b.DeviceId, b.UserId)
	if err != nil {
		return fmt.Errorf("Could not get upload keys parameters: %s", err)
	}

	_, err = b.client.EndToEndEncryption.UploadKeys(uploadKeys, b)
	if err != nil {
		return fmt.Errorf("Could not upload keys: %s", err)
	}

	err = b.Olm.MarkPublished()
	if err != nil && !strings.Contains(err.Error(), "read-only") {
		return fmt.Errorf("Error publishing: %s", err)
	}

	return nil
}

// Join a room.
func (b *Bot) JoinRoom(c context.Context, room_id string) error {
	joinParams := room_membership.NewJoinRoomByIDParamsWithContext(c)
	joinParams.SetRoomID(room_id)

	_, err := b.client.RoomMembership.JoinRoomByID(joinParams, b)
	return err
}

// Get a list of all members in a room.
func (b *Bot) GetRoomMembers(c context.Context, room_id string) ([]string, error) {
	roomParams := room_participation.NewGetMembersByRoomParamsWithContext(c)
	roomParams.SetRoomID(room_id)

	roomMembers, err := b.client.RoomParticipation.GetMembersByRoom(roomParams, b)
	if err != nil {
		return nil, fmt.Errorf("Error fetching room members: %s", err)
	}

	members := []string{}

	for _, chunk := range roomMembers.Payload.Chunk {
		members = append(members, chunk.GetMembersByRoomOKBodyChunkItemsAllOf0.GetMembersByRoomOKBodyChunkItemsAllOf0AllOf0.GetMembersByRoomOKBodyChunkItemsAllOf0AllOf0AllOf0.GetMembersByRoomOKBodyChunkItemsAllOf0AllOf0AllOf0AllOf0.GetMembersByRoomOKBodyChunkItemsAllOf0AllOf0AllOf0AllOf0AllOf0.StateKey)
	}

	return members, nil
}

// Claim a one-time key from each member of the room so that we can send them a group
// session key.
func (b *Bot) ClaimRoomMemberKeys(c context.Context, room_id string) (*models.ClaimKeysOKBody, models.QueryKeysOKBodyDeviceKeys, error) {
	deviceKeys := models.QueryKeysOKBodyDeviceKeys{}

	members, err := b.GetRoomMembers(c, room_id)
	if err != nil {
		return nil, deviceKeys, err
	}

	wantedDeviceKeys := map[string][]string{}
	for _, destId := range members {
		wantedDeviceKeys[destId] = []string{}
	}

	queryParams := end_to_end_encryption.NewQueryKeysParamsWithContext(c)
	queryParams.SetQuery(&models.QueryKeysParamsBody{
		DeviceKeys: wantedDeviceKeys,
	})

	query, err := b.client.EndToEndEncryption.QueryKeys(queryParams, b)
	if err != nil {
		return nil, deviceKeys, fmt.Errorf("Error fetching keys: %s", err)
	}

	deviceKeys = query.Payload.DeviceKeys
	wantedKeys := map[string]map[string]string{}

	for _, destId := range members {
		for destDeviceId, _ := range deviceKeys[destId] {
			if b.shookDevices[destDeviceId] {
				continue
			}

			if wantedKeys[destId] == nil {
				wantedKeys[destId] = map[string]string{}
			}

			wantedKeys[destId][destDeviceId] = "signed_curve25519"
		}
	}

	claimParams := end_to_end_encryption.NewClaimKeysParamsWithContext(c)
	claimParams.SetQuery(&models.ClaimKeysParamsBody{
		OneTimeKeys: wantedKeys,
	})

	claim, err := b.client.EndToEndEncryption.ClaimKeys(claimParams, b)
	if err != nil {
		return nil, deviceKeys, fmt.Errorf("Error claiming keys: %s", err)
	}

	return claim.Payload, deviceKeys, nil
}

// Initialize an outbound group session for a room and send the session key to every
// member of the channel so that we can send encrypted events.
func (b *Bot) HandshakeRoom(c context.Context, room_id string) error {
	oneTimeKeys, deviceKeys, err := b.ClaimRoomMemberKeys(c, room_id)
	if err != nil {
		return err
	}

	for destId, destDevices := range oneTimeKeys.OneTimeKeys {
		for destDeviceId, keys := range destDevices {
			for _, keyData := range keys {
				destKey := deviceKeys[destId][destDeviceId].Keys[fmt.Sprintf("curve25519:%s", destDeviceId)]
				ed25519Key := deviceKeys[destId][destDeviceId].Keys[fmt.Sprintf("ed25519:%s", destDeviceId)]
				destOneTimeKey := keyData.Key

				session := libolm.CreateOutboundSession(b.Olm.GetAccount(), destKey, destOneTimeKey)

				b.sessions = append(b.sessions, libolm.UserSession{
					Session:    session,
					UserId:     destId,
					DeviceId:   destDeviceId,
					DeviceKey:  destKey,
					Ed25519Key: ed25519Key,
				})

				b.shookDevices[destDeviceId] = true
			}
		}
	}

	return b.SendToDeviceEncrypted(c, map[string]interface{}{
		"algorithm":   "m.megolm.v1.aes-sha2",
		"room_id":     room_id,
		"session_id":  b.groupSession.GetSessionID(),
		"session_key": b.groupSession.GetSessionKey(),
	})
}

// Craft an encrypted event payload that can be sent to the server as an event.
func (b *Bot) EncryptedEvent(c context.Context, session libolm.Encrypter, event interface{}) (map[string]string, error) {
	contentEncoded, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("Error encoding content: %s", err)
	}

	_, ciphertext := session.Encrypt(string(contentEncoded))

	return map[string]string{
		"algorithm":  "m.megolm.v1.aes-sha2",
		"sender_key": b.Olm.GetIdentityKeys().Curve25519,
		"device_id":  b.DeviceId,
		"session_id": session.GetSessionID(),
		"ciphertext": ciphertext,
	}, nil
}

// Send an event to a channel.
func (b *Bot) SendEvent(c context.Context, channel string, eventType string, payload interface{}) error {
	params := room_participation.NewSendMessageParamsWithContext(c)
	params.SetRoomID(channel)
	params.SetBody(payload)
	params.SetEventType(eventType)

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

// Send an encrypted event to a channel.
func (b *Bot) SendEncryptedEvent(c context.Context, channel string, eventType string, message interface{}) error {
	if err := b.HandshakeRoom(c, channel); err != nil {
		return err
	}

	payload := map[string]interface{}{
		"type":    eventType,
		"content": message,
		"room_id": channel,
	}

	encrypted, err := b.EncryptedEvent(c, b.groupSession, payload)
	if err != nil {
		return fmt.Errorf("Could not encrypt event: %s", err)
	}

	return b.SendEvent(c, channel, "m.room.encrypted", encrypted)
}

// Send an unencrypted message to a channel.
func (b *Bot) Send(c context.Context, channel, message string) error {
	err := b.JoinRoom(c, channel)
	if err != nil {
		return err
	}

	unformatted, err := html2text.FromString(message, html2text.Options{})
	if err != nil {
		return err
	}

	return b.SendEvent(c, channel, "m.room.message", map[string]string{
		"msgtype":        "m.text",
		"formatted_body": message,
		"format":         "org.matrix.custom.html",
		"body":           unformatted,
	})
}

// Send an encrypted message to a channel.
func (b *Bot) SendEncrypted(c context.Context, channel, message string) error {
	err := b.JoinRoom(c, channel)
	if err != nil {
		return err
	}

	unformatted, err := html2text.FromString(message, html2text.Options{})
	if err != nil {
		return err
	}

	return b.SendEncryptedEvent(c, channel, "m.room.message", map[string]string{
		"body":           unformatted,
		"formatted_body": message,
		"msgtype":        "m.text",
		"format":         "org.matrix.custom.html",
	})
}

// Craft an encrypted event that can be sent directly to a device.
func (b *Bot) EncryptedDirectEvent(session libolm.UserSession, event interface{}) (*DirectEvent, error) {
	contentEncoded, err := json.Marshal(map[string]interface{}{
		"content":   event,
		"type":      "m.room_key",
		"sender":    b.UserId,
		"recipient": session.UserId,
		"keys": map[string]string{
			"ed25519": b.Olm.GetIdentityKeys().Ed25519,
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
		SenderKey: b.Olm.GetIdentityKeys().Curve25519,
		Ciphertext: map[string]map[string]interface{}{
			session.DeviceKey: map[string]interface{}{
				"body": ciphertext,
				"type": 0,
			},
		},
	}, nil
}

// Send a message to a specific device. Primarily used for sending room_key messages.
func (b *Bot) SendToDeviceEncrypted(c context.Context, event interface{}) error {
	params := send_to_device_messaging.NewSendToDeviceParamsWithContext(c)
	params.SetEventType("m.room.encrypted")

	messages := map[string]map[string]interface{}{}

	for _, session := range b.sessions {
		encrypted, err := b.EncryptedDirectEvent(session, event)
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

	_, err = b.client.SendToDeviceMessaging.SendToDevice(params, b)
	if err != nil {
		return fmt.Errorf("Could not send message: %s", err)
	}

	return nil
}
