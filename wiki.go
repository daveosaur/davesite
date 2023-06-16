/* things to do:
add search feature?
comments!
*/

package main

import (
    "html/template"
    "errors"
    "regexp"
    "os"
    "flag"
    "fmt"
    "log"
    "strings"
    "net/http"
    //"crypto/tls"

    "database/sql"
    _ "github.com/go-sql-driver/mysql"
    
)

type Data struct { //data to fill out templates
    Files []string
}

type Page struct { //wiki page
    Title string
    Body []byte
}
type Comment struct {
    Article string
    Name string
    Body []byte
}

//globals
var templates = template.Must(template.ParseFiles("edit.html", "view.html", "index.html", "newpage.html", "comment.html", "comment_edit.html", "footer.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|delete|new)/?([a-zA-Z0-9_]+)?$")
var validTitle = regexp.MustCompile("^([a-zA-Z0-9_]+)$")
var db *sql.DB
var log_file *os.File

//page functions
func getPage(name string) (*Page, error) {
    var title []byte
    var body []byte

    row := db.QueryRow("SELECT title, content FROM pages WHERE title = ?", name)
    err := row.Scan(&title, &body)
    if err != nil {
        return &Page{}, err
    }

    return &Page{Title: string(title), Body: body}, nil
}

func (p *Page) getComments() ([]Comment, error) {
    var name, body []byte
    comments := []Comment{}
    
    rows, err := db.Query("SELECT author, content FROM comments WHERE article = ?", p.Title)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()
    for rows.Next() {
        if err := rows.Scan(&name, &body); err != nil {
            log.Fatal(err)
        }
        comments = append(comments, Comment{Name: string(name), Body: body})
    }

    return comments, nil
}

func (c *Comment) save() error {
    if len(c.Article) == 0 || len(c.Name) == 0 || len(c.Body) == 0 {
        return fmt.Errorf("failed to save")
    }
    _, err := db.Exec("INSERT INTO comments (article, author, content) values (?, ?, ?)", c.Article, c.Name, c.Body)
    if err != nil {
        log.Fatal(err)
    }
    return nil
}

func (p *Page) save() error {
    if len(p.Title) == 0 || len(p.Body) == 0 {
        return fmt.Errorf("failed to save")
    }
    p.Title = strings.ReplaceAll(p.Title, " ", "_")
    _, err := db.Exec("INSERT INTO pages (title, content) VALUES (?, ?)", p.Title, p.Body)
    if err != nil {
        db.Exec("UPDATE pages SET content = ? WHERE title = ?", p.Body, p.Title)
        return nil
    }
    return nil
}

//misc functions?
func getTitle(w http.ResponseWriter, r *http.Request) (string, error) {
    m := validPath.FindStringSubmatch(r.URL.Path)
    if m == nil {
        //http.NotFound(w, r)
        return "", errors.New("invalid page title")
    }
    return m[2], nil
}
/*
func validateTitle(s string) error {
    if validTitle.Match([]byte(s)) {
        return nil
    }
    return fmt.Errorf("invalid")
}
*/


func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
    err := templates.ExecuteTemplate(w, tmpl+".html", p)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}

func renderComment(w http.ResponseWriter, tmpl string, c *Comment) {
    err := templates.ExecuteTemplate(w, tmpl+".html", c)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}


//net handlers
func viewHandler(w http.ResponseWriter, r *http.Request) {
    //fmt.Fprintln(log_file, r)
    title, err := getTitle(w, r)
    if err != nil {
        http.NotFound(w, r)
        return
    }
    p, err := getPage(title)
    if err != nil {
        http.Redirect(w, r, "/edit/"+title, http.StatusFound)
        return
    }
    renderTemplate(w, "view", p)

    c, _ := p.getComments()
    for _, comment := range c {
        renderComment(w, "comment", &comment)
    }
    renderTemplate(w, "comment_edit", p)
    renderTemplate(w, "footer", p)
}

func editHandler(w http.ResponseWriter, r *http.Request) {
    //fmt.Fprintln(log_file, r)
    title, err := getTitle(w, r)
    if err != nil {
        return
    }
    if title == "" {
        renderTemplate(w, "newpage", &Page{})
        return
    }
    p, err := getPage(title)
    if err != nil {
        p.Title = title
    }
    renderTemplate(w, "edit", p)
}


func saveHandler(w http.ResponseWriter, r *http.Request) {
    r.ParseForm()
    switch r.Form.Get("Type") {
    case "Article":
        p := &Page{Title: r.Form.Get("Title"), Body: []byte(r.Form.Get("Body"))}
        err := p.save()
        if err != nil {
            http.Redirect(w, r, "/", http.StatusFound)
            return
        }
        http.Redirect(w, r, "/view/"+p.Title, http.StatusFound)
    case "Comment":
        c := &Comment{Article: r.Form.Get("Article"), Name: r.Form.Get("Name"), Body: []byte(r.Form.Get("Body"))}
        err := c.save()
        _ = err //lol error checking
        http.Redirect(w, r, "/view/"+c.Article, http.StatusFound)
    default:
        return
    }
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
    title, err := getTitle(w, r)
    if err != nil {
        a := r.FormValue("title")
        if a == "" {
            http.NotFound(w, r)
            return
        }
        title = a
    }
    _ , err = db.Exec("DELETE FROM pages WHERE title = ?", title)
    if err != nil {
        log.Fatal(err)
    }
    _, err = db.Exec("DELETE FROM comments WHERE article = ?", title)
    if err != nil {
        log.Fatal(err)
    }

    http.Redirect(w, r, "/", http.StatusFound)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
    //fmt.Fprintln(log_file, r)
    rows, err := db.Query("SELECT title FROM pages")
    if err != nil {
        log.Fatal(err)
    }
    names := Data{}
    for rows.Next() {
        var title string
        rows.Scan(&title)
        names.Files = append(names.Files, title)
    }

    err = templates.ExecuteTemplate(w, "index.html", names)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}

func init() {
    var err error
    log_file, err = os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Fatal(err)
    }
}

func main() {
    tls := flag.Bool("tls", false, "enable https")
    flag.Parse()
    defer log_file.Close()
    fmt.Println("starting up")
    pass := os.Getenv("db_pass")
    var err error
    db, err = sql.Open("mysql", "dave:" + pass + "@tcp(localhost:3306)/wiki")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()
    fmt.Println("database connected")


    http.HandleFunc("/view/", viewHandler)
    http.HandleFunc("/edit/", editHandler)
    http.HandleFunc("/new/", editHandler)
    http.HandleFunc("/save/", saveHandler)
    http.HandleFunc("/delete/", deleteHandler)
    http.HandleFunc("/", indexHandler)

    if *tls {
        cert := "/etc/letsencrypt/live/wiki.dave.quest/cert.pem"
        key := "/etc/letsencrypt/live/wiki.dave.quest/privkey.pem"

        fmt.Println("using TLS")
        log.Fatal(http.ListenAndServeTLS(":3000", cert, key, nil))
    } else {
        log.Fatal(http.ListenAndServe(":3000", nil))
    }
}
