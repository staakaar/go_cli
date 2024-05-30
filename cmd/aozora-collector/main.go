package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/PuerkitoBio/goquery"
)

type Entry struct {
	AuthorID string
	Author   string
	TitleID  string
	Title    string
	InfoURL  string
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

	doc.Find("ol li a").Each(func(n int, elem *goquery.Selection) {
		println(elem.Text(), elem.AttrOr("href", ""))
	})

	return nil, nil
}

func main() {
	listURL := "https://www.aozora.gr.jp/index_pages/person879.html"

	entries, err := findEntries(listURL)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		fmt.Println(entry.Title, entry.ZipURL)
	}
}
