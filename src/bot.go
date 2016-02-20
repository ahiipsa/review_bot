package main

import (
    "./crucible"
    "./slack"
//    "./compare"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "reflect"
    "strings"
    "sync"
    "time"
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

    log.Println("Run Review Bot")

    log.Println("Чтение конфига...")
    config, err := getConfig()

    if err != nil {
        log.Fatalf("Ошибка чтения конфига:\n %s \n", err)
        return
    }

    globalConfig = config

    log.Println("Конфиг получен")

    slackClient, err := slack.CreateClient(config.Slack)

    if err != nil {
        log.Fatalf("Ошибка создания Slack клиента %s \n", err)
        return
    }

    err = slackClient.TestAuth()

    if err != nil {
        log.Fatalf("Ошибка не удалось авторизаваться с Slack, %s\n", err)
    }

    crucibleClient, err := crucible.CreateClient(config.Crucible)

    if err != nil {
        log.Fatalf("Ошибка в конфиге:\n %s\n", err)
        return
    }

    var wg sync.WaitGroup
    wg.Add(1)


    // Рассылка сообщений в Slack
    go func(){
        defer wg.Done()

        for {
            event := <-reviewEvents

            n := event.NewRev
            o := event.OldRev
            mTemplate := ""
            author := MapUserNicks([]string{event.NewRev.GetAuthorNick()})
            reviewers := MapUserNicks(event.NewRev.GetReviewersNames())

            equal, diff := o.Compare(n)

            log.Println("compare", equal, diff)

            if n.IsOpen() && !o.IsOpen() {
                mTemplate = "%[2]s нужно ревью"
            }

            if n.IsCompleted() && !o.IsCompleted() {
                mTemplate = "%[1]s ревью завершен"
            }

            if err != nil {
                fmt.Println("Ошибка Compare", err)
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
                event.NewRev.GetURL(config.Crucible.Host),
                event.OldRev.GetState(), event.NewRev.GetState(),
                fmt.Sprintf(mTemplate, author, reviewers),
            )

            if mTemplate == "" {
                continue
            }

            channelName, err := config.Crucible.GetChannelName(event.ProjectName)

            if err != nil {
                channelName = config.Slack.GetChannelName()
            }

            slackMessage := slack.Message{
                Text: fmt.Sprintf(mTemplate, author, reviewers),
                Channel: channelName,
            }

            title := n.Name;
            if title == "" {
                title = n.GetID()
            }

            slackMessage.AddAttachment(slack.Attachment{
                AuthorName: author,
                Title:      title,
                TitleLink:  event.NewRev.GetURL( config.Crucible.Host ),
            })

            err = slackClient.PostMessage(slackMessage)

            if err != nil {
                log.Printf("Ошибка отправки сообщения %s\n", err.Error())
            }
        }
    }()

    for _, project := range config.Crucible.Projects {
        log.Println("Подключаем проект", project.Name)
        wg.Add(1)
        go watchProject(project.Name, crucibleClient, reviewEvents, wg)
    }

    wg.Wait()

}

func watchProject(projectName string, crucibleClient crucible.Crucible, eventChannel chan ReviewEvent, wg sync.WaitGroup) {
    defer wg.Done()
    timeout := globalConfig.Crucible.Timeout
    reviews, err := crucibleClient.GetReviews(projectName)

    if err != nil {
        log.Printf("Ошибка получения списка review:\n %s\n", err.Error())
        return
    }

    log.Println("Получили список ревью", projectName, len(reviews.Reviews))
    count := 0
    updateError := false
    //
    for {
        time.Sleep(timeout * time.Second)
        update, err := crucibleClient.GetReviews(projectName)

        if err != nil {
            log.Printf("Ошибка обновления списка review %s:\n %s\n", projectName, err.Error())
            updateError = true
            continue
        } else if updateError {
            log.Printf("Успешно обновлён список review %s после ошибки", projectName)
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
