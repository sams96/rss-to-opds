package main

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/bmaupin/go-epub"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
	"github.com/opds-community/libopds2-go/opds1"
)

func feed(w http.ResponseWriter, r *http.Request) {
	url := r.PathValue("url")
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURL(url)

	opds := opds1.Feed{
		ID:      uuid.NewString(),
		Title:   feed.Title,
		Updated: time.Now(),
	}

	for _, item := range feed.Items {
		pub := opds1.Entry{
			Title:   item.Title,
			ID:      item.GUID,
			Updated: item.UpdatedParsed,
			Links: []opds1.Link{
				{
					Rel:      "http://opds-spec.org/acquisition/buy",
					TypeLink: "application/epub+zip",
					Href:     fmt.Sprintf("/%s/download/%s", url, item.GUID),
				},
			},
		}
		opds.Entries = append(opds.Entries, pub)
		opds.TotalResults++
		opds.ItemsPerPage++
	}

	j, _ := xml.Marshal(opds)

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	fmt.Fprintf(w, "%s", string(j))
}

func download(w http.ResponseWriter, r *http.Request) {
	url := r.PathValue("url")
	fp := gofeed.NewParser()
	feed, _ := fp.ParseURL(url)

	item := &gofeed.Item{}
	for _, i := range feed.Items {
		if i.GUID == r.PathValue("id") {
			item = i
		}
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(item.Content))
	if err != nil {
		log.Fatal(err)
	}

	doc.Find("br").Remove()
	content, err := doc.Html()
	if err != nil {
		log.Fatal(err)
	}

	e := epub.NewEpub(item.Title)
	e.SetAuthor(item.Authors[0].Name)
	e.AddSection(content, "", "firstsection.html", "")

	fmt.Println(content)

	pw := bufio.NewWriterSize(w, 10<<16)

	w.Header().Set("Content-Type", "application/epub+zip")
	_, err = e.WriteTo(pw)
	if err != nil {
		log.Fatal(err)
	}

	pw.Flush()
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/{url}", feed)
	mux.HandleFunc("/{url}/download/{id}", download)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
