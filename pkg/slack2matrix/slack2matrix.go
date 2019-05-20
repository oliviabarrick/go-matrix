package slack2matrix

import (
	"encoding/json"
	"fmt"
	"github.com/russross/blackfriday"
	"gopkg.in/go-playground/colors.v1"
	"net/url"
	"regexp"
	"strings"
)

var (
	urlRe = regexp.MustCompile("<(.*?)[|](.*?)>")
	pRe   = regexp.MustCompile("(?s)^<p>(.*?)</p>$")
)

// Represents a slack message sent to the API
type SlackMessage struct {
	Channel     string            `json:"channel"`
	Color       string            `json:"color"`
	IconEmoji   string            `json:"icon_emoji"`
	Username    string            `json:"username"`
	Text        MarkdownString    `json:"text"`
	Title       MarkdownString    `json:"title"`
	Attachments []SlackAttachment `json:"attachments"`
}

// Represents a section of a slack message that is sent to the API
type SlackAttachment struct {
	Color     string         `json:"color"`
	Title     MarkdownString `json:"title"`
	TitleLink MarkdownString `json:"title_link"`
	Text      MarkdownString `json:"text"`
	Fields    []SlackFields  `json:"fields"`
}

type SlackFields struct {
	Value MarkdownString `json:"value"`
}

type MarkdownString string

func (m MarkdownString) ReplaceLinks() string {
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

	color, err := ColorSpan(s.Color)
	if err != nil {
		return "", err
	}

	body := ""
	if mainTitle != "" {
		body = fmt.Sprintf("<div>%s<b>%s</b></div>", color, mainTitle)
		color = ""
	}

	if mainText != "" {
		body = fmt.Sprintf("%s<div>%s%s</div>", body, color, mainText)
	}

	for _, attachment := range s.Attachments {
		attachmentBody, err := attachment.ToHTML()
		if err != nil {
			return "", err
		}
		body += attachmentBody
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

	color, err := ColorSpan(s.Color)

	body := ""
	if mainTitle != "" {
		body = fmt.Sprintf("<div>%s<b>%s</b></div>", color, mainTitle)
		color = ""
	}

	if mainText != "" {
		body = fmt.Sprintf("%s<div>%s%s</div>", body, color, mainText)
	}

	for _, field := range s.Fields {
		fieldStr, err := field.Value.ToHTML()
		if err != nil {
			return "", err
		}

		body = fmt.Sprintf("%s<div>%s%s</div>", body, color, fieldStr)
	}

	return body, err
}

func ColorSpan(color string) (string, error) {
	span := ""

	knownColors := map[string]string{
		"danger":  "#a30200",
		"warning": "#d69d38",
		"good":    "#33cc99",
	}

	if color != "" {
		if knownColors[color] != "" {
			color = knownColors[color]
		}

		parsedColor, err := colors.Parse(color)
		if err != nil {
			return "", err
		}

		hexColor := parsedColor.ToRGB().ToHEX()
		span = fmt.Sprintf("<span data-mx-bg-color=\"%s\">&nbsp;</span>&nbsp;", hexColor)
	}

	return span, nil
}
