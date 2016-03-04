package main

import (
    "./crucible"
    "./slack"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "reflect"
    "strings"
    "sync"
    "time"
    "golang.org/x/net/websocket"
)

// Документация https://docs.atlassian.com/fisheye-crucible/latest/wadl/crucible.html

var globalConfig Config

type Config struct {
    Crucible crucible.Config   `json:"crucible"`
    Slack    slack.Config      `json:"slack"`
    UserMap  map[string]string `json:"usermap"`
}

func getConfig() (config Config, err error) {
    data, err := ioutil.ReadFile("config.json")
    if err != nil {
        return
    }

    err = json.Unmarshal(data, &config)
    if err != nil {
        return
    }

    return
}

func MapUserNicks(names []string) string {

    mentions := []string{}

    for _, value := range names {
        nick := globalConfig.UserMap[value]
        if nick == "" {
            nick = value
        }
        mentions = append(mentions, fmt.Sprintf("@%s", nick))
    }

    return strings.Join(mentions, ", ")
}

type ReviewEvent struct {
    ProjectName string
    OldRev crucible.Review
    NewRev crucible.Review
}

func main() {
    reviewEvents := make(chan ReviewEvent)

    log.Println("Старт бота")
    log.Println("Чтение конфига...")
    var err error
    globalConfig, err = getConfig()

    if err != nil {
        log.Fatalln("Ошибка чтения конфига", err)
        return
    }

    log.Println("Конфиг получен")

    slackClient, err := slack.CreateClient(globalConfig.Slack)

    if err != nil {
        log.Fatalln("Ошибка создания Slack клиента", err)
        return
    }

    err = slackClient.TestAuth()

    if err != nil {
        log.Fatalln("Не удалось авторизаваться с Slack", err)
    }

    crucibleClient, err := crucible.CreateClient(globalConfig.Crucible)

    if err != nil {
        log.Fatalln("Не удалось создать Crucible клиент", err)
    }

    var wg sync.WaitGroup

    wg.Add(1)
    watchCommand(&slackClient, &crucibleClient, &wg)

    // Рассылка сообщений в Slack
    go listenReviewUpdate(reviewEvents, slackClient)

    for _, project := range globalConfig.Crucible.Projects {
        log.Println("Подключаем проект", project.Name)
        wg.Add(1)
        go watchProject(project.Name, &crucibleClient, reviewEvents, &wg)
    }

    wg.Wait()
}

/*
    Слежение за списком ревью, при обновлении ревью посылает событие в канал `eventChannel chan ReviewEvent`
 */
func watchProject(projectName string, crucibleClient *crucible.Crucible, eventChannel chan ReviewEvent, wg *sync.WaitGroup) {
    defer wg.Done()
    timeout := globalConfig.Crucible.Timeout
    reviews, err := crucibleClient.GetReviews(crucible.GetReviewsOptions{
        Project: projectName,
        FromDate: time.Now().AddDate(0, 0, -7), // weekago
    })

    if err != nil {
        log.Fatalln("Ошибка получения списка review", err)
        return
    }

    log.Println("Получили список ревью", projectName, len(reviews.Reviews))
    count := 0
    updateError := false

    for {
        time.Sleep(timeout * time.Second)
        update, err := crucibleClient.GetReviews(crucible.GetReviewsOptions{
            Project: projectName,
            FromDate: time.Now().AddDate(0, 0, -7), // weekago
        })

        if err != nil {
            log.Println("Ошибка обновления списка review", projectName, err)
            updateError = true
            continue
        } else if updateError {
            log.Println("Успешно обновлён список review после ошибки", projectName)
            updateError = false
        }

        for _, renewed := range update.Reviews {
            old, _ := reviews.FindById(renewed.GetID())

            // Новое ревью или обновилось
            if !reflect.DeepEqual(renewed, old) {
                eventChannel <- ReviewEvent{
                    ProjectName: projectName,
                    NewRev: renewed,
                    OldRev: old,
                }
            }
        }

        reviews = update
        count++
    }
}

func listenReviewUpdate(reviewEvents chan ReviewEvent, slackClient slack.SlackClient){

    for {
        event := <-reviewEvents

        n := event.NewRev
        o := event.OldRev
        mTemplate := ""
        author := MapUserNicks([]string{event.NewRev.GetAuthorNick()})
        reviewers := MapUserNicks(event.NewRev.GetReviewersNames())

        equal, diff := crucible.Compare(o, n)

        log.Println("compare", equal, diff)

        if n.IsOpen() && !o.IsOpen() {
            mTemplate = "%[2]s нужно ревью"
        }

        if n.IsCompleted() && !o.IsCompleted() {
            mTemplate = "%[1]s ревью завершен"
        }


        log.Printf(
`Обновление:
id: %s
name: %s
author: %s
url: %s
status: %s -> %s
template %s`,
            event.NewRev.GetID(),
            event.NewRev.Name,
            author,
            event.NewRev.GetURL(globalConfig.Crucible.Host),
            event.OldRev.GetState(), event.NewRev.GetState(),
            fmt.Sprintf(mTemplate, author, reviewers),
        )

        if mTemplate == "" {
            continue
        }

        channelName, err := globalConfig.Crucible.GetChannelName(event.ProjectName)

        if err != nil {
            log.Println("Не указан канал для проекта", event.ProjectName)
            channelName = globalConfig.Slack.GetChannelName()
        }

        if channelName == "" {
            log.Println("Не канал по умолчанию", event.ProjectName)
        }

        slackMessage := slack.Message{
            Text: fmt.Sprintf(mTemplate, author, reviewers),
            Channel: channelName,
            IconUrl: "http://lorempixel.com/48/48/cats/",
            AsUser: false,
        }

        title := n.Name
        if title == "" {
            title = n.GetID()
        }

        slackMessage.AddAttachment(slack.Attachment{
            AuthorName: author,
            Title:      title,
            TitleLink:  event.NewRev.GetURL(globalConfig.Crucible.Host),
        })

        err = slackClient.PostMessage(slackMessage)

        if err != nil {
            log.Println("Ошибка отправки сообщения", err)
        }
    }
}

type SlackMessage struct {
    Type    string `json:"type"`
    Subtype string `json:"subtype"`
    Channel string `json:"channel"`
    User    string `json:"user"`
    Text    string `json:"text"`
    Time    string `json:"ts"`
}

/**
    Обрабатывае сообщения из слака и преобразует в команды
 */
func watchCommand(slackClient *slack.SlackClient, crucibleClient *crucible.Crucible, wg *sync.WaitGroup) {
    defer wg.Done()

    go func() {
        rtmStart, err := slackClient.RTMStart()

        if err != nil {
            log.Fatalln("Ошибка получения настроек для RTM", err)
            return
        }

        ws, err := websocket.Dial(rtmStart.Url, "", "http://localhost/")

        if err != nil {
            log.Fatalln("Ошибка websocket соединения", err)
            return
        }


        for {
            var messageRaw []byte
            var message SlackMessage
            err = websocket.Message.Receive(ws, &messageRaw)
            if err != nil {
                log.Println("Ошибка получения сообщения из Slack", err)
                continue
            }

            err = json.Unmarshal(messageRaw, &message)
            if err != nil {
                log.Println("Ошибка парсинга в JSON", err)
                continue
            }

            log.Println("Сообщение", string(messageRaw[:]))

            if strings.Contains(message.Text, "reviews 1234") {
                log.Println("Выполняем команду...")
                slackClient.PostMessage(slack.Message{
                    Channel: message.Channel,
                    Text: "Минутку...",
                });

                continue
                reviews, err := crucibleClient.GetReviews(crucible.GetReviewsOptions{
                    States: []string{"Review"},
                });

                if err != nil {
                    log.Println("ошибка получения ревью:", err)
                    continue
                }

                list := slack.Message{
                    Text: "Список незакрытых ревью",
                    Channel: message.Channel,
                    IconUrl: "http://lorempixel.com/48/48/cats/",
                    AsUser: false,
                }

                // Сформировать сообщение со списком открытых ревью
                for _, rev := range reviews.Reviews {
                    attachment := slack.Attachment{
                        TitleLink: rev.GetURL(globalConfig.Crucible.Host),
                        AuthorName: MapUserNicks([]string{rev.GetAuthorNick()}),
                    }

                    attachment.Title = rev.Name;
                    if attachment.Title == "" {
                        attachment.Title = rev.GetID()
                    }

                    attachment.Color = "good"

                    if !rev.IsCompleted() {
                        attachment.Color = "danger" // red
                    }

                    list.AddAttachment(attachment)
                    channelName, err := globalConfig.Crucible.GetChannelName(rev.ProjectKey)
                    log.Println("channel", channelName, err)
                }

                slackClient.PostMessage(list);
                log.Println("Отправили список...")

            }

        }
    }()
}
