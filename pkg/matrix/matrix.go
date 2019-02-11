package matrix

import (
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/justinbarrick/go-matrix/pkg/client"
	"github.com/justinbarrick/go-matrix/pkg/client/room_participation"
	"github.com/justinbarrick/go-matrix/pkg/client/room_membership"
	"github.com/justinbarrick/go-matrix/pkg/client/session_management"
	"github.com/justinbarrick/go-matrix/pkg/client/end_to_end_encryption"
	"github.com/justinbarrick/go-matrix/pkg/client/send_to_device_messaging"
	"github.com/justinbarrick/go-matrix/pkg/models"

	"encoding/json"
	"strings"
	"fmt"
	libolm "github.com/justinbarrick/libolm-go"
)

// Represents an encrypted event that will be sent directly to a device.
type DirectEvent struct {
	Algorithm string `json:"algorithm"`
	SenderKey string `json:"sender_key"`
	SessionId string `json:"session_id,omitempty"`
	DeviceId  string `json:"device_id,omitempty"`
	Ciphertext map[string]map[string]interface{} `json:"ciphertext"`
}

// A bot instance that can send messages to Matrix channels.
type Bot struct {
	userId     string
	deviceId     string
	accessToken string
	username    string
	password    string
	olm         *libolm.Matrix
	client      *client.MatrixClientServer
}

// Initialize a new bot instance. Most provide either username+password or accessToken.
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
	} else if accessToken == "" {
		return bot, fmt.Errorf("username and password or accessToken must be provided.")
	}

	var err error

	bot.olm, err = libolm.NewMatrix()
	if err != nil {
		return bot, err
	}

	err = bot.UploadKeys()
	if err != nil {
		return bot, err
	}

	return bot, nil
}

// Implement ClientAuthInfoWriter by adding the bot's access token to all API requests.
func (b *Bot) AuthenticateRequest(request runtime.ClientRequest, registry strfmt.Registry) error {
	if b.accessToken == "" {
		return fmt.Errorf("No access token set, please login.")
	}

	request.SetQueryParam("access_token", b.accessToken)
	return nil
}

// Login with a username and password, not needed if accessToken is provided.
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

// Get the current user id associated with our token.
func (b *Bot) GetUserId() (string, error) {
	if b.userId != "" {
		return b.userId, nil
	}

	owner, err := b.client.UserData.GetTokenOwner(nil, b)
	if err != nil {
		return "", err
	}

	b.userId = *owner.Payload.UserID
	return b.userId, nil
}

// Get the current device ID that we are using.
func (b *Bot) GetDeviceId() (string, error) {
	if b.deviceId != "" {
		return b.deviceId, nil
	}

	devices, err := b.client.DeviceManagement.GetDevices(nil, b)
	if err != nil {
		return "", err
	}

	deviceId := ""

	for _, device := range devices.Payload.Devices {
		deviceId = *device.DeviceID
	}

	b.deviceId = deviceId
	return deviceId, nil
}

// Upload our keys to the server.
func (b *Bot) UploadKeys() error {
	deviceId, err := b.GetDeviceId()
	if err != nil {
		return fmt.Errorf("Could not get device id: %s", err)
	}

	userId, err := b.GetUserId()
	if err != nil {
		return fmt.Errorf("Could not get user id: %s", err)
	}

	uploadKeys, err := b.olm.UploadKeysParams(deviceId, userId)
	if err != nil {
		return fmt.Errorf("Could not get upload keys parameters: %s", err)
	}

	_, err = b.client.EndToEndEncryption.UploadKeys(uploadKeys, b)
	if err != nil {
		return fmt.Errorf("Could not upload keys: %s", err)
	}

	err = b.olm.MarkPublished()
	if err != nil && ! strings.Contains(err.Error(), "read-only") {
		return fmt.Errorf("Error publishing: %s", err)
	}

	return nil
}

// Join a room.
func (b *Bot) JoinRoom(room_id string) error {
	joinParams := room_membership.NewJoinRoomByIDParams()
	joinParams.SetRoomID(room_id)

	_, err := b.client.RoomMembership.JoinRoomByID(joinParams, b)
	return err
}

// Get a list of all members in a room.
func (b *Bot) GetRoomMembers(room_id string) ([]string, error) {
	roomParams := room_participation.NewGetMembersByRoomParams()
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
func (b *Bot) ClaimRoomMemberKeys(room_id string) (*models.ClaimKeysOKBody, models.QueryKeysOKBodyDeviceKeys, error) {
	deviceKeys := models.QueryKeysOKBodyDeviceKeys{}

	members, err := b.GetRoomMembers(room_id)
	if err != nil {
		return nil, deviceKeys, err
	}

	wantedDeviceKeys := map[string][]string{}
	for _, destId := range members {
		wantedDeviceKeys[destId] = []string{}
	}

	queryParams := end_to_end_encryption.NewQueryKeysParams()
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
			if wantedKeys[destId] == nil {
				wantedKeys[destId] = map[string]string{}
			}

			wantedKeys[destId][destDeviceId] = "signed_curve25519"
		}
	}

	claimParams := end_to_end_encryption.NewClaimKeysParams()
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
func (b *Bot) HandshakeRoom(room_id string) (libolm.GroupSession, error) {
	groupSession := libolm.CreateOutboundGroupSession()

	oneTimeKeys, deviceKeys, err := b.ClaimRoomMemberKeys(room_id)
	if err != nil {
		return groupSession, err
	}

	sessions := []libolm.UserSession{}

	for destId, destDevices := range oneTimeKeys.OneTimeKeys {
		for destDeviceId, keys := range destDevices {
			for _, keyData := range keys {
				destKey := deviceKeys[destId][destDeviceId].Keys[fmt.Sprintf("curve25519:%s", destDeviceId)]
				ed25519Key := deviceKeys[destId][destDeviceId].Keys[fmt.Sprintf("ed25519:%s", destDeviceId)]
				destOneTimeKey := keyData.Key

				session := libolm.CreateOutboundSession(b.olm.GetAccount(), destKey, destOneTimeKey)

				sessions = append(sessions, libolm.UserSession{
					Session: session,
					UserId: destId,
					DeviceId: destDeviceId,
					DeviceKey: destKey,
					Ed25519Key: ed25519Key,
				})
			}
		}
	}

	return groupSession, b.SendToDeviceEncrypted(sessions, map[string]interface{}{
		"algorithm": "m.megolm.v1.aes-sha2",
		"room_id": room_id,
		"session_id": groupSession.GetSessionID(),
		"session_key": groupSession.GetSessionKey(),
	})
}

// Craft an encrypted event payload that can be sent to the server as an event.
func (b *Bot) EncryptedEvent(session libolm.Encrypter, event interface{}) (map[string]string, error) {
	deviceId, err := b.GetDeviceId()
	if err != nil {
		return nil, fmt.Errorf("Could not get device id: %s", err)
	}

	contentEncoded, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("Error encoding content: %s", err)
	}

	_, ciphertext := session.Encrypt(string(contentEncoded))

	return map[string]string{
		"algorithm": "m.megolm.v1.aes-sha2",
		"sender_key": b.olm.GetIdentityKeys().Curve25519,
		"device_id": deviceId,
		"session_id": session.GetSessionID(),
		"ciphertext": ciphertext,
	}, nil
}

// Send an event to a channel.
func (b *Bot) SendEvent(channel string, eventType string, payload interface{}) error {
	params := room_participation.NewSendMessageParams()
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
func (b *Bot) SendEncryptedEvent(session libolm.Encrypter, channel string, eventType string, message interface{}) error {
	payload := map[string]interface{}{
		"type": eventType,
		"content": message,
		"room_id": channel,
	}

	encrypted, err := b.EncryptedEvent(session, payload)
	if err != nil {
		return fmt.Errorf("Could not encrypt event: %s", err)
	}

	return b.SendEvent(channel, "m.room.encrypted", encrypted)
}

// Send an unencrypted message to a channel.
func (b *Bot) Send(channel, message string) error {
	err := b.JoinRoom(channel)
	if err != nil {
		return err
	}

	return b.SendEvent(channel, "m.room.message", map[string]string{
		"msgtype": "m.text",
		"formatted_body": message,
		"format": "org.matrix.custom.html",
		"body": message,
	})
}

// Send an encrypted message to a channel.
func (b *Bot) SendEncrypted(channel, message string) error {
	err := b.JoinRoom(channel)
	if err != nil {
		return err
	}

	groupSession, err := b.HandshakeRoom(channel)
	if err != nil {
		return err
	}

	return b.SendEncryptedEvent(groupSession, channel, "m.room.message", map[string]string{
		"body": message,
		"formatted_body": message,
		"msgtype": "m.text",
		"format": "org.matrix.custom.html",
	})
}

// Craft an encrypted event that can be sent directly to a device.
func (b *Bot) EncryptedDirectEvent(session libolm.UserSession, event interface{}) (*DirectEvent, error) {
	userId, err := b.GetUserId()
	if err != nil {
		return nil, fmt.Errorf("Could not get user id: %s", err)
	}

	contentEncoded, err := json.Marshal(map[string]interface{}{
		"content": event,
		"type": "m.room_key",
		"sender": userId,
		"recipient": session.UserId,
		"keys": map[string]string{
			"ed25519": b.olm.GetIdentityKeys().Ed25519,
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
		SenderKey: b.olm.GetIdentityKeys().Curve25519,
		Ciphertext: map[string]map[string]interface{}{
			session.DeviceKey: map[string]interface{}{
				"body": ciphertext,
				"type": 0,
			},
		},
	}, nil
}

// Send a message to a specific device. Primarily used for sending room_key messages.
func (b *Bot) SendToDeviceEncrypted(sessions []libolm.UserSession, event interface{}) error {
	params := send_to_device_messaging.NewSendToDeviceParams()
	params.SetEventType("m.room.encrypted")

	messages := map[string]map[string]interface{}{}

	for _, session := range sessions {
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
