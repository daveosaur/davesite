/* things to do:
add search feature?
*/

package main

import (
    "html/template"
    "errors"
    "regexp"
    "os"
    "fmt"
    "log"
    "strings"
    "net/http"

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

//globals
var templates = template.Must(template.ParseFiles("edit.html", "view.html", "index.html", "newpage.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|delete|new)/?([a-zA-Z0-9_]+)?$")
var validTitle = regexp.MustCompile("^([a-zA-Z0-9_]+)$")
var db *sql.DB

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

func (p *Page) save() error {
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
func validateTitle(s string) error {
    if validTitle.Match([]byte(s)) {
        return nil
    }
    return fmt.Errorf("invalid")
}


func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
    //p.Title = strings.ReplaceAll(p.Title, "_", " ")
    err := templates.ExecuteTemplate(w, tmpl+".html", p)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}

//net handlers
func viewHandler(w http.ResponseWriter, r *http.Request) {
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
}

func editHandler(w http.ResponseWriter, r *http.Request) {
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
    title, err := getTitle(w, r)
    if err != nil {
        return
    }
    if title == "" {
        title = strings.ReplaceAll(r.FormValue("title"), " ", "_")
        err := validateTitle(title)
        if err != nil {
            http.NotFound(w, r)
            return
        }
    }
    
    body := r.FormValue("body")
    p := &Page{Title: title, Body: []byte(body)}
    err = p.save()
    if err != nil {
        log.Fatal(err)
        /*
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
        */
    }
    http.Redirect(w, r, "/view/"+p.Title, http.StatusFound)
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
    http.Redirect(w, r, "/", http.StatusFound)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
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

func main() {
    fmt.Println("starting up")
    pass := os.Getenv("db_pass")
    var err error
    db, err = sql.Open("mysql", "dave:" + pass + "@tcp(127.0.0.1:3306)/wiki")
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

    log.Fatal(http.ListenAndServe(":3000", nil))
}
