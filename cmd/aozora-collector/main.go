package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/ikawaha/kagome/tokenizer"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/text/encoding/japanese"
)

type Entry struct {
	AuthorID string
	Author   string
	TitleID  string
	Title    string
	InfoURL  string
	SiteUrl  string
	ZipURL   string
}

func findEntries(siteURL string) ([]Entry, error) {
	response, err := http.Get("https://example.com")
	if err != nil {
		fmt.Println("Error:", err)
		return nil, nil
	}
	defer response.Body.Close()

	// ステータスコードをチェック
	if response.StatusCode != 200 {
		fmt.Printf("Error: Status code %d\n", response.StatusCode)
		return nil, nil
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, err
	}

	pat := regexp.MustCompile(`.*/cards/([0-9]+)/card([0-9]+).html$`)

	entries := []Entry{}
	doc.Find("ol li a").Each(func(n int, elem *goquery.Selection) {
		println(elem.Text(), elem.AttrOr("href", ""))
		token := pat.FindStringSubmatch(elem.AttrOr("href", ""))
		if len(token) != 3 {
			return
		}
		pageURL := fmt.Sprintf("https://www.aozora.gr.jp/cards/%s/card%s.html", token[1], token[2])
		println(pageURL)

		author, zipURL := findAuthorAndZIP(pageURL)

		if zipURL != "" {
			entries = append(entries, Entry{
				AuthorID: token[1],
				Author:   author,
				TitleID:  token[2],
				Title:    title,
				SiteURL:  siteURL,
				ZipURL:   zipURL,
			})
		}
	})

	return entries, nil
}

func findAuthorAndZIP(siteURL string) (string, string) {
	response, err := http.Get(siteURL)
	if err != nil {
		fmt.Println("Error:", err)
		return "", ""
	}
	defer response.Body.Close()

	// ステータスコードを確認
	if response.StatusCode != 200 {
		fmt.Printf("Error: Status code %d\n", response.StatusCode)
		return "", ""
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return "", ""
	}

	author := doc.Find("table[summary=作家データ] tr:nth-child(1) td:nth-child(2)").Text()

	zipURL := ""
	doc.Find("table.download a").Each(func(n int, elem *goquery.Selection) {
		href := elem.AttrOr("href", "")
		if strings.HasSuffix(href, ".zip") {
			zipURL = href
		}
	})

	if zipURL == "" {
		return author, ""
	}
	if strings.HasPrefix(zipURL, "http://") || strings.HasPrefix(zipURL, "https://") {
		return author, zipURL
	}

	u, err := url.Parse(siteURL)
	if err != nil {
		return author, ""
	}

	u.Path = path.Join(path.Dir(u.Path), zipURL)

	return author, u.String()
}

func extractText(zipURL string) (string, error) {
	res, err := http.Get(zipURL)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", err
	}

	for _, file := range r.File {
		if path.Ext(file.Name) == ".txt" {
			f, err := file.Open()
			if err != nil {
				return "", err
			}

			b, err := ioutil.ReadAll(f)
			f.Close()
			if err != nil {
				return "", err
			}

			b, err = japanese.ShiftJIS.NewDecoder().Bytes(b)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	return "", errors.New("contents nout found")
}

func setupDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS authors(author_id TEXT, author TEXT, PRIMARY KEY(author_id))`,
		`CREATE TABLE IF NOT EXISTS contents(author_id TEXT, title_id TEXT, title TEXT, content TEXT, PRIMARY KEY(author_id, title_id))`,
		`CREATEVIRTUAL TABLE IF NOT EXSITS contents_fts USING fts(words)`,
	}

	for _, query := range queries {
		_, err = db.Exec(query)
		if err != nil {
			return nil, err
		}
	}
	return db, nil
}

func addEntry(db *sql.DB, entry *Entry, content string) error {
	_, err := db.Exec(`REPLACE INTO authors(author_id, author) values (?, ?)`, entry.AuthorID, entry.Author)
	if err != nil {
		return err
	}

	res, err := db.Exec(`REPLACE INTO contents(author_id, title_id, title, content) values(?, ?, ?, ?)`, entry.AuthorID, entry.TitleID, entry.Title, content)
	if err != nil {
		return err
	}

	docID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		return err
	}

	seg := t.Wakati(content)
	_, err = db.Exec(`REPLACE INTO contents_fts(docid, words) values(?,?)`, docID, strings.Join(seg, " "))
	if err != nil {
		return err
	}
	return nil
}

func main() {
	// db, err := setupDB("database.sqlite")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// defer db.Close()

	db, err := sql.Open("sqlite3", "database.sqlite")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS contents(author_id TEXT, author TEXT, PRIMARY KEY(author_id))`,
		`CREATE TABLE IF NOT EXISTS contents(author_id TEXT, title_id TEXT, title TEXT, content TEXT, PRIMARY KEY(author_id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS contents_fts USING fts(words)`,
	}

	for _, query := range queries {
		_, err = db.Exec(query)
		if err != nil {
			log.Fatal(err)
		}
	}

	b, err := os.ReadFile("text.txt")
	if err != nil {
		log.Fatal(err)
	}

	b, err = japanese.ShiftJIS.NewDecoder().Bytes(b)
	if err != nil {
		log.Fatal(err)
	}

	content := string(b)
	res, err := db.Exec(`INSERT INTO contents(author_id, title_id, title, content) values (?, ?, ?, ?)`, "000001", "11", "test", content)
	if err != nil {
		log.Fatal(err)
	}

	docID, err := res.LastInsertId()

	t, err := tokenizer.New(ipa.Dict(), tokenizer.OmitBosEos())
	if err != nil {
		log.Fatal(err)
	}

	seg := t.Wakati(content)

	_, err = db.Exec(`INSERT INTO contents_fts(docid, words) values (?, ?)`, docID, strings.Join(seg, " "))
	if err != nil {
		log.Fatal(err)
	}

	query := "虫 AND ココア"
	rows, err := db.Query(`
		SELECT a.author, c.title 
		FROM contents c 
		INNER JOIN authors a ON a.author_id = c.author_id 
		INNER JOIN contents_fts f ON c.rowid = f.docid AND words MATCH ?`, query)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		var author, title string
		err = rows.Scan(&author, &title)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(author, title)
	}

	listURL := "https://www.aozora.gr.jp/index_pages/person879.html"

	entries, err := findEntries(listURL)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("found %d entries", len(entries))
	for _, entry := range entries {
		log.Printf("adding %+v\n", entry)
		content, err := extractText(entry.ZipURL)
		if err != nil {
			log.Println(err)
			continue
		}

		err = addEntry(db, &entry, content)
		if err != nil {
			log.Println(err)
			continue
		}

		fmt.Println(entry.SiteUrl)
		fmt.Println(content)
	}

	for _, entry := range entries {
		fmt.Println(entry.Title, entry.ZipURL)
	}
}
