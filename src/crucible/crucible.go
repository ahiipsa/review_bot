package crucible

import (
    "net/url"
    "net/http"
    "fmt"
    "strings"
    "io/ioutil"
    "encoding/json"
    "errors"
    "time"
    "strconv"
    "reflect"
)


type CrucibleToken struct {
    Token string `json:"token"`
}

type Config struct {
    Host string `json:"host"`
    Login string `json:"login"`
    Password string `json:"password"`
    Timeout time.Duration `json:"timeout"`
}

type Crucible struct {
    url *url.URL
    httpClient *http.Client
    config Config
    token string
}

func CreateClient(config Config) (client Crucible, err error) {
    client.httpClient = &http.Client{
        Timeout: time.Duration(10 * time.Second),
    }
    client.config = config
    client.url, err = url.Parse(config.Host)
    if err != nil {
        return
    }

    return
}


func (client *Crucible) getUrl() url.URL {
    return *client.url
}


func (client *Crucible) requestToken() (token CrucibleToken, err error) {
    apiUrl := client.getUrl()
    apiUrl.Path = "/rest-service-fecru/auth/login"

    form := url.Values{}
    form.Add("userName", client.config.Login)
    form.Add("password", client.config.Password)

    req, err := http.NewRequest("POST", apiUrl.String(), strings.NewReader(form.Encode()))

    if err != nil {
        return
    }

    res, err := client.httpClient.Do(req)

    if err != nil {
        return
    }

    if res.StatusCode > 200 {
        err = errors.New(fmt.Sprint("Не удалось получить токен ", res.Status))
        return
    }

    bytes, err := ioutil.ReadAll(res.Body)

    if err != nil {
        return
    }

    err = json.Unmarshal(bytes, &token);
    return
}

func (client *Crucible) GetToken() (token string, err error) {
    if client.token != "" {
        token = client.token
        return
    }

    tok, err := client.requestToken()

    if err != nil {
        return
    }

    if tok.Token == "" {
        err = errors.New("Не удалось получить токен")
        return
    }

    client.token = tok.Token
    token = tok.Token
    return
}

type GetReviewsOptions struct {
    Project string
    FromDate time.Time
    States []string
}

func (client *Crucible) GetReviews(options GetReviewsOptions) (reviewList ReviewList, err error) {
    apiUrl := client.getUrl()
    apiUrl.Path = "/rest-service/reviews-v1/filter/details"

    token, err := client.GetToken()

    if err != nil {
        return
    }

    query := apiUrl.Query()
    query.Set("FEAUTH", token)

    if options.Project != "" {
        query.Set("project", options.Project)
    }

    if !options.FromDate.IsZero() {
        fromDate := options.FromDate.UnixNano() / int64(time.Millisecond)
        query.Set("fromDate", strconv.FormatInt(fromDate, 10))
    }


    if len(options.States) > 0 {
        query.Set("states", strings.Join(options.States, ","))
    }

    apiUrl.RawQuery = query.Encode()

    request, err := http.NewRequest("GET", apiUrl.String(), nil)
    request.Header.Set("Accept", "application/json")

    if err != nil {
        return
    }

    httpClient := http.Client{}

    response, err := httpClient.Do(request)

    if err != nil {
        return
    }

    defer response.Body.Close()

    bytes, err := ioutil.ReadAll(response.Body)

    if err != nil {
        return
    }

    err = json.Unmarshal(bytes, &reviewList)
    return
}

/* Review */

type Review struct {
    Author struct {
                AvatarURL   string `json:"avatarUrl"`
                DisplayName string `json:"displayName"`
                URL         string `json:"url"`
                UserName    string `json:"userName"`
            } `json:"author"`
    CreateDate string `json:"createDate"`
    Creator    struct {
                AvatarURL   string `json:"avatarUrl"`
                DisplayName string `json:"displayName"`
                URL         string `json:"url"`
                UserName    string `json:"userName"`
            } `json:"creator"`
    Description     string `json:"description"`
    GeneralComments struct {
                Comments []interface{} `json:"comments"`
            } `json:"generalComments"`
    JiraIssueKey   string `json:"jiraIssueKey"`
    MetricsVersion int    `json:"metricsVersion"`
    Name           string `json:"name"`
    PermaID        struct {
                ID string `json:"id"`
            } `json:"permaId"`
    PermaIDHistory []string `json:"permaIdHistory"`
    ProjectKey     string   `json:"projectKey"`
    Reviewers      struct {
                Reviewer []struct {
                    AvatarURL                  string `json:"avatarUrl"`
                    Completed                  bool   `json:"completed"`
                    CompletionStatusChangeDate int    `json:"completionStatusChangeDate"`
                    DisplayName                string `json:"displayName"`
//                    TimeSpent                  int    `json:"timeSpent"`
                    UserName                   string `json:"userName"`
                } `json:"reviewer"`
            } `json:"reviewers"`
    State string `json:"state"`
    Type string `json:"type"`
}

func Compare(v1 Review, v2 Review) (equal bool, diffs []string) {

    if reflect.DeepEqual(v1, v2) {
        return equal, diffs
    }

    if v1.Name != v2.Name {
        diffs = append(diffs, "name")
    }

    if v1.State != v2.State {
        diffs = append(diffs, "state")
    }

    if v1.Description != v2.Description {
        diffs = append(diffs, "description")
    }

    if !reflect.DeepEqual(v1.Reviewers, v2.Reviewers) {
        v1Completed := v1.GetCountCompleted()
        v2Completed := v2.GetCountCompleted()

        if v1Completed < v2Completed {
            diffs = append(diffs, "reviewers.complited")
        }

        if len(v1.Reviewers.Reviewer) < len(v2.Reviewers.Reviewer) {
            diffs = append(diffs, "reviewers.join")
        }

        if len(v1.Reviewers.Reviewer) > len(v2.Reviewers.Reviewer) {
            diffs = append(diffs, "reviewers.leave")
        }
    }

    equal = len(diffs) == 0
    return equal, diffs
}

func (review *Review) IsCompleted() bool {
    return review.GetCountCompleted() >= 2
}


func (review *Review) GetID() string {
    return review.PermaID.ID
}


func (review *Review) GetCountCompleted() int {
    count := 0

    for _, reviewer := range review.Reviewers.Reviewer {
        if reviewer.Completed == true {
            count++
        }
    }

    return count
}


func (review *Review) GetState() string {
    return review.State
}


func (review *Review) IsOpen() bool {
    return review.GetState() == "Review"
}


func (review *Review) GetURL(host string) string {
    return fmt.Sprintf("%s/cru/%s", host, review.GetID())
}


func (review *Review) GetAuthorNick() string {
    return review.Author.UserName
}


func (review *Review) GetAuthorName() string {
    return review.Author.DisplayName
}


func (review *Review) GetReviewersNames() (names []string) {
    for _, reviewer := range review.Reviewers.Reviewer {
        names = append(names, reviewer.UserName)
    }

    return
}


/* ReviewList */

type ReviewList struct {
    Reviews []Review `json:"detailedReviewData"`
}

func (reviews *ReviewList) Filter(f func(Review) bool) []Review {
    resultList := make([]Review, 0)

    for _, ireview := range reviews.Reviews {
        if f(ireview) {
            resultList = append(resultList, ireview)
        }
    }

    return resultList;
}

func (reviews *ReviewList) FindById(id string) (review Review, err error) {
    arr := reviews.Filter(func(item Review) bool {
        if(item.GetID() == id){
            return true
        } else {
            return false;
        }
    })

    if len(arr) > 0 {
        review = arr[0]
    } else {
        err = errors.New("Not found")
    }

    return
}