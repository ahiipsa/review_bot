package slack

import (
    "net/url"
    "net/http"
    "fmt"
    "errors"
    "encoding/json"
    "strings"
    "io/ioutil"
)


type Config struct  {
    Host string `json:"host"`
    Token string `json:"token"`
    Channel string
}

func (config *Config) GetChannelName() string {
    return config.Channel
}


type SlackClient struct {
    url *url.URL
    httpClient *http.Client
    config Config
    token string
}


func CreateClient(config Config) (client SlackClient, err error) {
    client.httpClient = &http.Client{}
    client.config = config
    client.url, err = url.Parse(config.Host)

    query := client.url.Query()
    query.Set("token", config.Token)
    // query.Set("pretty", "1")
    client.url.RawQuery = query.Encode()

    if err != nil {
        return
    }

    return
}


type Message struct {
    Text string `json:"text"`
    Channel string `json:"channel"`
    Attachments []Attachment
    IconUrl string `json:"icon_url"`
    AsUser bool `json:"as_user"`
}


func (message *Message) AddAttachment(attachment Attachment) {
    message.Attachments = append(message.Attachments, attachment);
}


type Attachment struct {
    Fallback string `json:"fallback"`
    Pretext string `json:"pretext"`
    Title string `json:"title"`
    TitleLink string `json:"title_link"`
    Text string `json:"text"`
    Color string `json:"color"`
    AuthorName string `json:"author_name"`
    Fields []AttachmentField `json:"fields"`
}


type AttachmentField struct {
    Title string `json:"title"`
    Value string `json:"value"`
    Short bool `json:"short"`
}

func (client *SlackClient) getUrl() url.URL {
    return *client.url
}


func (client *SlackClient) TestAuth() (err error) {
    url := client.getUrl()
    url.Path = "api/auth.test"

    req, err := http.NewRequest("GET", url.String(), nil)

    if err != nil {
        return
    }

    response, err := client.httpClient.Do(req)

    if err != nil {
        return
    }

    if response.StatusCode > 200 {
        err = errors.New("Slack: неудалось авторизоваться")
        return
    }

    return
}


func (client *SlackClient) PostMessage(message Message) (err error) {
    urlAPI := client.getUrl()
    urlAPI.Path = "/api/chat.postMessage"

    data, err := json.Marshal(message.Attachments)

    if err != nil {
        fmt.Println(err)
    }

    form := url.Values{}
    form.Add("channel", message.Channel)
    form.Add("text", message.Text)
    form.Add("attachments", string(data[:]))
    form.Add("parse", "full")
    form.Add("link_names", "1")
    form.Add("username", "BotReview")
    req, err := http.NewRequest("POST", urlAPI.String(), strings.NewReader(form.Encode()))

    if err != nil {
        return
    }

    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    res, err := client.httpClient.Do(req)

    if err != nil {
        return
    }

    body, err := ioutil.ReadAll(res.Body)

    if res.StatusCode > 200 {
        err = errors.New(fmt.Sprint("Slack: не удалось отправить сообщение", string(body[:])))
        return
    }

    return
}

// https://api.slack.com/methods/rtm.start
type RTMStart struct {
    Ok bool `json:"ok"`
    Url string `json:"url"`
    Channels []Channel `json:"channels"`
    Groups []Channel `json:"groups"`
}


func (rtmStart *RTMStart) ChannelName(id string) string {
    for _, channel := range rtmStart.Channels {
        if channel.ID == id {
            return channel.Name
        }
    }

    for _, channel := range rtmStart.Groups {
        if channel.ID == id {
            return channel.Name
        }
    }

    return ""
}

// https://api.slack.com/types/channel
type Channel struct {
    ID string `json:"id"`
    Name string `json:"name"`
}

func (client *SlackClient) RTMStart() (rtmStart RTMStart, err error) {
    url := client.getUrl()
    url.Path = "api/rtm.start"

    req, err := http.NewRequest("GET", url.String(), nil)

    if err != nil {
        return
    }

    response, err := client.httpClient.Do(req)

    if err != nil {
        return
    }

    defer response.Body.Close()

    if response.StatusCode > 200 {
        err = errors.New("Slack. не удалось получить url")
        return
    }

    bodyBytes, err := ioutil.ReadAll(response.Body)

    if err != nil {
        return
    }

    err = json.Unmarshal(bodyBytes, &rtmStart)

    if err != nil {
        return
    }

    return
}
