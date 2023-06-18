/* things to do:
add search feature?
user accounts --kinda done
*/

package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/crypto/scrypt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
    "time"

	"database/sql"
	_ "github.com/go-sql-driver/mysql"
)

type Data struct { //data to fill out templates
    Logged bool
    Name string
	Files []string
}

type Page struct { //wiki page
    Logged bool
	Title string
	Body  []byte
}
type Comment struct { //wiki comment
    Logged bool
	Title string
	Name  string
	Body  []byte
}

type Element interface { //pointer to page/comment
	save() error
}

// globals
var templates = template.Must(template.ParseFiles("templates/edit.html", "templates/view.html", "templates/index.html", "templates/newpage.html", "templates/comment.html", "templates/comment_edit.html", "templates/footer.html", "templates/signup.html"))
var validPath = regexp.MustCompile("^/(edit|save|view|delete|new|signup|login)/?([a-zA-Z0-9_]+)?$")
var SESSION_LENGTH = (time.Minute * 24)

// var validTitle = regexp.MustCompile("^([a-zA-Z0-9_]+)$")  //deprecated?
var db *sql.DB

// page functions
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
	if len(c.Title) == 0 || len(c.Name) == 0 || len(c.Body) == 0 {
		return fmt.Errorf("failed to save")
	}
	_, err := db.Exec("INSERT INTO comments (article, author, content) values (?, ?, ?)", c.Title, c.Name, c.Body)
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

//session functions?
func isLogged(w http.ResponseWriter, r *http.Request) (bool, string) {
    var logged bool
    var name string
    cookie, err := checkCookie(w, r)
    if err != nil {
        log.Fatal(err)
    }
    row := db.QueryRow("SELECT loggedIn, name FROM sessions WHERE session = ?", cookie.Value)
    err = row.Scan(&logged, &name)
    if err != nil {
        return false, ""
    }
    return logged, name
}

func checkCookie(w http.ResponseWriter, r *http.Request) (*http.Cookie, error) {
    cookie, err := r.Cookie("daveWiki")             //grab cookie if it exists
    if err != nil {     //doesnt exist!
        switch {
        case errors.Is(err, http.ErrNoCookie):
            token := make([]byte, 16)
            _, err := rand.Read(token)      //generate token
            if err != nil {
                log.Fatal(err)
            }
            session := base64.StdEncoding.EncodeToString(token)
            expiration := time.Now().Add(SESSION_LENGTH).Unix()
            cookie := http.Cookie {
                Name: "daveWiki",
                Value: session,
                MaxAge: 3600,
                Path: "/",
                Secure: true,
                SameSite: http.SameSiteLaxMode,
            }
            http.SetCookie(w, &cookie)

            _, err = db.Exec("INSERT INTO sessions (loggedIn, name, session, expiration) values (?, ?, ?, ?)", 0, "", session, expiration)
            if err != nil {
                log.Fatal(err)
            }

            return &cookie, nil
        default:        //oh something actually fucked up?
            return nil, err
        }
    }
    return cookie, nil
}

// misc functions?
func getTitle(w http.ResponseWriter, r *http.Request) (string, error) { //can probably kill this soon?
	m := validPath.FindStringSubmatch(r.URL.Path)
	if m == nil {
		return "", errors.New("invalid page title")
	}
	return m[2], nil //     m[2] would be the article string i guess.
}

func renderElement(w http.ResponseWriter, tmpl string, e Element) {
	err := templates.ExecuteTemplate(w, tmpl+".html", e)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// user functions
func saveUser(w http.ResponseWriter, r *http.Request) error {
	hash := make([]byte, 10)
	_, err := rand.Read(hash)
	if err != nil {
		return fmt.Errorf("failed to seed hash")
	}
	newhash := base64.StdEncoding.EncodeToString(hash)
	pass, err := scrypt.Key([]byte(r.Form.Get("Pass")), []byte(newhash), 1<<15, 8, 1, 64)
	newpass := base64.StdEncoding.EncodeToString(pass)
	_, err = db.Exec("INSERT INTO users (name, hash, pass) values (?, ?, ?)", r.Form.Get("Name"), newhash, newpass)
	if err != nil {
		return fmt.Errorf("failed to save user")
	}
	return nil
}

func authUser(w http.ResponseWriter, r *http.Request) (string, error) {
	r.ParseForm()
	var hash, pass string
    name := r.Form.Get("Name")
	row := db.QueryRow("SELECT hash, pass FROM users WHERE name = ?", name)
	err := row.Scan(&hash, &pass)
	if err != nil {
		return "", fmt.Errorf("no user found")
	}
	mess, err := scrypt.Key([]byte(r.Form.Get("Pass")), []byte(hash), 1<<15, 8, 1, 64)
	comparePass := base64.StdEncoding.EncodeToString(mess)
    if string(comparePass) == pass { //hooray correct password
        return name, nil
    }
    return "", fmt.Errorf("invalid user/password") //boo not correct password
}

// net handlers
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
    //add a header to render at some point.
	renderElement(w, "view", p)
	c, _ := p.getComments()
	for _, comment := range c {
		renderElement(w, "comment", &comment)
	}
	renderElement(w, "comment_edit", p)
	renderElement(w, "footer", p)
}

func editHandler(w http.ResponseWriter, r *http.Request) {
    logged, _ := isLogged(w, r)
    if !logged {
        http.Redirect(w, r, "/", http.StatusFound)
        return
    }
	title, err := getTitle(w, r)
	if err != nil {
		return
	}
	if title == "" {
		renderElement(w, "newpage", &Page{})
		return
	}
	p, err := getPage(title)
	if err != nil {
		p.Title = title
	}
	renderElement(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	switch r.Form.Get("Type") {
	case "Article":
		p := &Page{Title: r.Form.Get("Title"), Body: []byte(r.Form.Get("Body"))}
		err := p.save()
		if err != nil {
            log.Println(err)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Redirect(w, r, "/view/"+p.Title, http.StatusFound)
	case "Comment":
		c := &Comment{Title: r.Form.Get("Article"), Name: r.Form.Get("Name"), Body: []byte(r.Form.Get("Body"))}
		err := c.save()
        if err != nil {
            log.Println(err)
			http.Redirect(w, r, "/", http.StatusFound)
            return
        }
		http.Redirect(w, r, "/view/"+c.Title, http.StatusFound)
	case "NewUser":
		err := saveUser(w, r)
		if err != nil {
			log.Fatal(err)
		}
		http.Redirect(w, r, "/", http.StatusFound) //make a success page
	default:
		//do something for fake http request
		return
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	auth, err := authUser(w, r)
	if err != nil {
        fmt.Println("failed!")
		http.Error(w, "invalid user/pass", http.StatusInternalServerError)
        return
	}
    fmt.Println("success!")
    expiration := time.Now().Add(SESSION_LENGTH).Unix()
    cookie, err := checkCookie(w, r)
    if err != nil {
        log.Fatal(err)
    }

    _, err = db.Exec("UPDATE sessions SET loggedIn = ? WHERE name = ?", 0, auth)
    if err != nil {
        log.Fatal(err)
    }
    result , err := db.Exec("UPDATE sessions SET loggedIn = ?, expiration = ?, name = ? WHERE session = ?", 1, expiration, auth, cookie.Value)
    if err != nil { //dupe users found
        log.Fatal(err)
    }
    updated, _ := result.RowsAffected()
    if updated < 1 {
        db.Exec("INSERT into sessions (loggedIn, name, expiration, session) values (?, ?, ?, ?)", 1, auth, expiration, cookie.Value)
    }

    http.Redirect(w, r, "/", http.StatusFound) //do a better login thing.
}

func deleteHandler(w http.ResponseWriter, r *http.Request) { //deletes page (and comments)
    logged, _ := isLogged(w, r)
    if !logged {
        http.Redirect(w, r, "/", http.StatusFound)
        return
    }
	title, err := getTitle(w, r)
	if err != nil {
		a := r.FormValue("title")
		if a == "" {
			http.NotFound(w, r)
			return
		}
		title = a
	}
	_, err = db.Exec("DELETE FROM pages WHERE title = ?", title)
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec("DELETE FROM comments WHERE article = ?", title)
	if err != nil {
		log.Fatal(err)
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {//render index
    rows, err := db.Query("SELECT title FROM pages")
	if err != nil {
		log.Fatal(err)
	}
    logged, name := isLogged(w, r)
    names := Data{Logged: logged, Name: name}
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

func signupHandler(w http.ResponseWriter, r *http.Request) {//render signup page
    _, err := checkCookie(w, r)
    if err != nil {
        log.Fatal(err)
    }

	title, err := getTitle(w, r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	_ = title
	renderElement(w, "signup", &Page{})
}

func main() {
	tls := flag.Bool("tls", false, "enable https")
    port := flag.String("port", "3000", "change the port used.")
	flag.Parse()
	fmt.Println("starting up")
	db_pass := os.Getenv("db_pass")
    cert_path  := os.Getenv("cert_path")
	var err error
	db, err = sql.Open("mysql", "dave:" + db_pass + "@tcp(localhost:3306)/wiki")
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
	http.HandleFunc("/signup/", signupHandler)
	http.HandleFunc("/login/", loginHandler)
	http.HandleFunc("/", indexHandler)

	if *tls {
		fmt.Println("using TLS")
        log.Fatal(http.ListenAndServeTLS(":" + *port, cert_path + "cert.pem", cert_path + "privkey.pem", nil))
	} else {
        fmt.Println("no TLS :/")
        log.Fatal(http.ListenAndServe(":" + *port, nil))
	}
}
