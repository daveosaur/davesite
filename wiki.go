package main

import (
    "html/template"
    "errors"
    "regexp"
    "os"
    "fmt"
    "log"
    "net/http"
)

type Data struct {
    Files []string
}

type Page struct {
    Title string
    Body []byte
}

//globals
var templates = template.Must(template.ParseFiles("edit.html", "view.html", "index.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|delete)/([a-zA-Z0-9]+)$")

//page functions
func (p *Page) save() error {
    filename := p.Title + ".txt"
    return os.WriteFile("pages/" + filename, p.Body, 0600)
}

func loadPage(title string) (*Page, error) {
    filename := title + ".txt"
    body, err := os.ReadFile("pages/" + filename)
    if err != nil {
        return nil, err
    }
    return &Page{Title: title, Body: body}, nil
}

//misc functions?
func getTitle(w http.ResponseWriter, r *http.Request) (string, error) {
    m := validPath.FindStringSubmatch(r.URL.Path)
    if m == nil {
        http.NotFound(w, r)
        return "", errors.New("invalid page title")
    }
    return m[2], nil
}

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
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
        return
    }
    p, err := loadPage(title)
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
    p, err := loadPage(title)
    if err != nil {
        p = &Page{Title: title}
    }
    renderTemplate(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
    title, err := getTitle(w, r)
    if err != nil {
        return
    }
    body := r.FormValue("body")
    p := &Page{Title: title, Body: []byte(body)}
    err = p.save()
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    http.Redirect(w, r, "/view/"+title, http.StatusFound)
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
    title, err := getTitle(w, r)
    if err != nil {
        return
    }
    err = os.Remove("pages/" + title + ".txt")
    if err != nil {
        log.Fatal(err)
    }
    http.Redirect(w, r, "/", http.StatusFound)
}
    

func indexHandler(w http.ResponseWriter, r *http.Request) {
    files, err := os.ReadDir("pages/")
    if err != nil {
        log.Fatal(err)
    }
    names := Data{}
    for _, file := range files {
        names.Files = append(names.Files, file.Name()[:len(file.Name())-4])
    }
    err = templates.ExecuteTemplate(w, "index.html", names)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}

func main() {
    fmt.Println("starting up")

    http.HandleFunc("/view/", viewHandler)
    http.HandleFunc("/edit/", editHandler)
    http.HandleFunc("/save/", saveHandler)
    http.HandleFunc("/delete/", deleteHandler)
    http.HandleFunc("/", indexHandler)

    log.Fatal(http.ListenAndServe(":3000", nil))
}
