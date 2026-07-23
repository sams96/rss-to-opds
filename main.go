package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sams96/rss-to-opds/epub"

	"github.com/antchfx/xmlquery"
	"github.com/davidbyttow/govips/v2/vips"
	"github.com/google/uuid"
	"golang.org/x/net/html"
)

type handler struct {
	log *slog.Logger
}

func fetchFeed(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d is not okay", resp.StatusCode)
	}

	return resp.Body, nil
}

type streamItem struct {
	node    *xmlquery.Node
	title   string
	guid    string
	updated string
}

func yieldStreamItems(stream io.Reader) iter.Seq2[streamItem, error] {
	return func(yield func(streamItem, error) bool) {
		parser, err := xmlquery.CreateStreamParser(stream, "/rss/channel/title | /rss/channel/item")
		if err != nil {
			yield(streamItem{}, err)
			return
		}

		for {
			node, err := parser.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				yield(streamItem{}, fmt.Errorf("error reading stream: %w", err))
				return
			}

			var item streamItem
			item.node = node

			if node.Data == "title" {
				item.title = node.InnerText()
				if !yield(item, nil) {
					return
				}
				continue
			}

			if n := xmlquery.FindOne(node, "./title"); n != nil {
				item.title = n.InnerText()
			}
			if n := xmlquery.FindOne(node, "./guid | ./id"); n != nil {
				item.guid = n.InnerText()
			}
			if n := xmlquery.FindOne(node, "./pubDate | ./updated"); n != nil {
				item.updated = n.InnerText()
			}

			if !yield(item, nil) {
				return
			}
		}
	}
}

func (h *handler) feed(w http.ResponseWriter, r *http.Request) {
	feedURL := r.PathValue("url")
	log := h.log.With(slog.String("feed", feedURL))

	resp, err := fetchFeed(r.Context(), feedURL)
	if err != nil {
		log.InfoContext(r.Context(), "failed to fetch feed", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer resp.Close()

	w.Header().Set("Content-Type", "application/atom+xml;profile=opds-catalog;kind=acquisition")
	w.WriteHeader(http.StatusOK)

	feedTitle := "untitled feed"
	headerWritten := false

	for item, err := range yieldStreamItems(resp) {
		if err != nil {
			// TODO: return an error in the feed
			log.ErrorContext(r.Context(), "error reading stream item", slog.String("error", err.Error()))
			return
		}

		if item.guid == "" {
			feedTitle = item.title
			continue
		}

		if !headerWritten {
			_, err = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:opds="http://opds-spec.org/2010/catalog">
  <id>%s</id>
  <title>%s</title>
  <updated>%s</updated>
  `, uuid.NewString(), html.EscapeString(feedTitle), time.Now().Format(time.RFC3339))
			if err != nil {
				log.ErrorContext(r.Context(), "failed to write header", slog.String("error", err.Error()))
				return
			}
			headerWritten = true
		}

		downloadHref := fmt.Sprintf("/%s/download/%s", url.QueryEscape(feedURL), url.QueryEscape(item.guid))

		_, err = fmt.Fprintf(w, `  <entry>
    <title>%s</title>
    <id>%s</id>
    <updated>%s</updated>
    <link rel="http://opds-spec.org/acquisition/buy" type="application/epub+zip" href="%s"/>
  </entry>
`, html.EscapeString(item.title), html.EscapeString(item.guid), item.updated, html.EscapeString(downloadHref))
		if err != nil {
			log.ErrorContext(r.Context(), "failed to write entry", slog.String("error", err.Error()))
			return
		}

	}

	if !headerWritten {
		fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>%s</title>`, html.EscapeString(feedTitle))
	}

	io.WriteString(w, "</feed>")
}

func replaceExt(filename, newExt string) string {
	ext := filepath.Ext(filename)
	name := strings.TrimSuffix(filename, ext)
	return name + newExt
}

type cdataFilterReader struct {
	r io.Reader
}

func (c *cdataFilterReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n == 0 {
		return n, err
	}

	data := p[:n]
	data = bytes.ReplaceAll(data, []byte("<![CDATA["), nil)
	data = bytes.ReplaceAll(data, []byte("]]>"), nil)

	copy(p, data)
	return len(data), err
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	feedURL := r.PathValue("url")
	id := r.PathValue("id")
	log := h.log.With(slog.String("feed", feedURL), slog.String("id", id))

	resp, err := fetchFeed(r.Context(), feedURL)
	if err != nil {
		log.InfoContext(r.Context(), "failed to fetch feed", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer resp.Close()

	var (
		targetItem *streamItem
		feedTitle  string
	)
	for item, err := range yieldStreamItems(resp) {
		if err != nil {
			// TODO: return an error in the feed
			log.ErrorContext(r.Context(), "error reading stream item", slog.String("error", err.Error()))
			return
		}

		if item.guid == "" {
			feedTitle = item.title
			continue
		}

		if item.guid == id {
			targetItem = &item
			break
		}
	}

	if targetItem == nil {
		log.InfoContext(r.Context(), "item missing")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var content io.Reader
	if n := xmlquery.FindOne(targetItem.node, "./content:encoded | ./content | ./description"); n != nil {
		content = &cdataFilterReader{r: streamContent(r.Context(), n)}
	}

	e, err := epub.New(targetItem.title, w)
	if err != nil {
		log.ErrorContext(r.Context(), "failed to create epub", slog.String("error", err.Error()))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	e.SetAuthor(feedTitle)

	var (
		tokeniser    = html.NewTokenizer(content)
		cleanHTML    bytes.Buffer
		h1TextBuf    strings.Builder
		inH1         bool
		sectionTitle = "Introduction"
		flushSection = func(title string) {
			if cleanHTML.Len() > 0 {
				e.AddSection(&cleanHTML, title)
				cleanHTML.Reset()
			}
		}
	)

loop:
	for {
		tokenType := tokeniser.Next()
		if tokenType == html.ErrorToken {
			err := tokeniser.Err()
			if err == io.EOF {
				break loop
			}
			log.WarnContext(r.Context(), "failed to tokenise html", slog.String("error", err.Error()))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		token := tokeniser.Token()

		switch tokenType {
		case html.StartTagToken, html.SelfClosingTagToken:
			switch token.Data {
			case "br", "hr":
				continue
			case "h1":
				flushSection(sectionTitle)
				inH1 = true
				h1TextBuf.Reset()
			case "img":
				token = h.replaceImage(r.Context(), e, token)
			}

		case html.EndTagToken:
			switch token.Data {
			case "br", "hr":
				continue
			case "h1":
				inH1 = false
				title := strings.TrimSpace(h1TextBuf.String())
				if title != "" {
					sectionTitle = title
				}
			}

		case html.TextToken:
			if inH1 {
				h1TextBuf.WriteString(token.Data)
			}
		}

		cleanHTML.WriteString(token.String())
	}

	if sectionTitle == "Introduction" {
		sectionTitle = targetItem.title
	}
	flushSection(sectionTitle)

	w.Header().Set("Content-Type", "application/epub+zip")
	_, err = e.Write()
	if err != nil {
		log.ErrorContext(r.Context(), "failed to write epub", slog.String("error", err.Error()))
		return
	}

	log.DebugContext(r.Context(), "successfully served article", slog.String("title", targetItem.title))
}

func streamContent(ctx context.Context, node *xmlquery.Node) io.Reader {
	pr, pw := io.Pipe()
	context.AfterFunc(ctx, func() {
		pr.Close()
	})

	go func() {
		err := node.Write(pw, false)
		_ = pw.CloseWithError(err)
	}()

	return pr
}

func (h *handler) replaceImage(ctx context.Context, e *epub.Epub, token html.Token) html.Token {
	token.Type = html.SelfClosingTagToken

	var originalSrc, altText string

	for _, attr := range token.Attr {
		switch attr.Key {
		case "src":
			originalSrc = attr.Val
		case "alt":
			altText = attr.Val
		}
	}

	if !strings.HasPrefix(originalSrc, "http") {
		token.Attr = []html.Attribute{{Key: "src", Val: originalSrc}}
		return token
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, originalSrc, nil)
	if err != nil {
		token.Attr = []html.Attribute{{Key: "src", Val: originalSrc}}
		return token
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		token.Attr = []html.Attribute{{Key: "src", Val: originalSrc}}
		return token
	}
	defer resp.Body.Close()

	pathParts := strings.Split(originalSrc, "/")
	filename := url.PathEscape(replaceExt(pathParts[len(pathParts)-1], ".jpeg"))

	transcoded := transcodeImage(ctx, resp.Body, &imageOptions{maxDimensions: new(1024), greyscale: true})
	e.AddMedia(transcoded, filename, "image/jpeg")

	newAttrs := []html.Attribute{{Key: "src", Val: filename}}
	if altText != "" {
		newAttrs = append(newAttrs, html.Attribute{Key: "alt", Val: altText})
	}

	token.Attr = newAttrs
	return token
}

func main() {
	vips.Startup(nil)
	defer vips.Shutdown()

	mux := http.NewServeMux()

	h := handler{
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})),
	}

	mux.HandleFunc("/{url}", h.feed)
	mux.HandleFunc("/{url}/download/{id}", h.download)

	log.Fatal(http.ListenAndServe(":8080", mux))
}
