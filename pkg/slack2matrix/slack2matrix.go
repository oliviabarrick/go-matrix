package slack2matrix

import (
	"encoding/json"
	"net/url"
	"fmt"
	"regexp"
	"strings"
	"github.com/russross/blackfriday"
)

var (
	urlRe = regexp.MustCompile("<(.*?)[|](.*?)>")
	pRe = regexp.MustCompile("(?s)^<p>(.*?)</p>$")
)

// Represents a slack message sent to the API
type SlackMessage struct {
	Channel     string            `json:"channel"`
	IconEmoji   string            `json:"icon_emoji"`
	Username    string            `json:"username"`
	Text        MarkdownString    `json:"text"`
	Title       MarkdownString    `json:"title"`
	Attachments []SlackAttachment `json:"attachments"`
}

// Represents a section of a slack message that is sent to the API
type SlackAttachment struct {
	Color     string `json:"color"`
	Title     MarkdownString `json:"title"`
	TitleLink MarkdownString `json:"title_link"`
	Text      MarkdownString `json:"text"`
}

type MarkdownString string

func (m MarkdownString) ReplaceLinks() (string) {
	links := urlRe.FindAllStringSubmatch(string(m), -1)

	final := string(m)

	for _, link := range links {
		url := link[1]
		title := link[2]

		final = strings.Replace(final, link[0], fmt.Sprintf("[%s](%s)", title, url), 1)
	}

	return final
}

func (m MarkdownString) ToHTML() (string, error) {
	body := strings.TrimRight(string(blackfriday.Run([]byte(m.ReplaceLinks()))), "\n")
	body = pRe.ReplaceAllString(body, "$1")
	return body, nil
}

func ParseSlackWebhook(body []byte) (SlackMessage, error) {
	message := SlackMessage{}

	values, err := url.ParseQuery(string(body))
	if err == nil && values.Get("payload") != "" {
		body = []byte(values.Get("payload"))
	}
		
	err = json.Unmarshal(body, &message)
	return message, err
}

func (s *SlackMessage) ToHTML() (string, error) {
	mainText, err := s.Text.ToHTML()
	if err != nil {
		return "", err
	}

	mainTitle, err := s.Title.ToHTML()
	if err != nil {
		return "", err
	}

	body := ""
	if mainTitle != "" {
		body = fmt.Sprintf("<h3>%s</h3>", mainTitle)
	}

	body = fmt.Sprintf("%s%s<br>", body, mainText)

	for _, attachment := range s.Attachments {
		attachmentBody, err := attachment.ToHTML()
		if err != nil {
			return "", err
		}
		body = fmt.Sprintf("%s<br>%s", body, attachmentBody)
	}

	return body, nil
}

func (s *SlackAttachment) ToHTML() (string, error) {
	mainText, err := s.Text.ToHTML()
	if err != nil {
		return "", err
	}

	mainTitle, err := s.Title.ToHTML()
	if err != nil {
		return "", err
	}

	body := ""
	if mainTitle != "" {
		body = fmt.Sprintf("<h4>%s</h4>", mainTitle)
	}

	return fmt.Sprintf("%s%s<br>", body, mainText), nil
}
