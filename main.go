package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/cli/go-gh/pkg/browser"
	"github.com/cli/go-gh/pkg/jsonpretty"
	"github.com/cli/go-gh/pkg/tableprinter"
	"github.com/cli/go-gh/pkg/term"
	"github.com/google/go-github/v47/github"
	"github.com/spf13/cobra"
)

const trendingBaseURL string = "https://github.com/trending"

var cutset *regexp.Regexp = regexp.MustCompile("(,|\n| |/)")

type outputFormat string

const (
	JSON  outputFormat = "json"
	table outputFormat = "table"
)

var (
	openInBrowser *bool
	outFormat     *string
)

func GetTrending(language string) ([]github.Repository, error) {
	res, err := http.Get(fmt.Sprintf("%s/%s", trendingBaseURL, language))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("received non-200 response from GitHub: %+v", res)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	trendings := []github.Repository{}
	doc.Find("article.Box-row").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Find("h1.h3.lh-condensed > a").Attr("href")
		if !exists {
			log.Printf("Did not find href attr on eleemnt %+v\n", s)
			return
		}

		ownerRepo := strings.Split(href, "/")
		if len(ownerRepo) != 3 {
			log.Printf("href / split is not 3 long: %+v", ownerRepo)
			return
		}

		trendings = append(trendings, github.Repository{
			Owner: &github.User{Login: &ownerRepo[1]},
			Name:  &ownerRepo[2],
			StargazersCount: func() *int {
				stargazersInt64, err := strconv.ParseInt(
					cutset.ReplaceAllString(
						s.Find(
							fmt.Sprintf("a[href=\"%s/stargazers\"]", href),
						).Text(),
						"",
					),
					10, 64,
				)
				if err != nil {
					panic(err)
				}
				stargazersInt := int(stargazersInt64)
				return &stargazersInt
			}(),
			Language: func() *string {
				var lang string
				if lang = cutset.ReplaceAllString(
					s.Find("span[itemprop=\"programmingLanguage\"]").Text(), "",
				); lang != "" {
					return &lang
				}
				return nil
			}(),
			Description: func() *string {
				var desc string
				if desc = strings.Trim(
					s.Find("p.col-9").Text(), "\n ",
				); desc != "" {
					return &desc
				}
				return nil
			}(),
		})
	})

	sort.Slice(trendings, func(i, j int) bool {
		return *(trendings[i].StargazersCount) > *(trendings[j].StargazersCount)
	})

	return trendings, nil
}

var trendingCmd cobra.Command = cobra.Command{
	Use:   "gh-trending (language?) ...",
	Short: "Show trending repositories",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if *openInBrowser && outputFormat(*outFormat) == JSON {
			log.Fatal("Cannot select output format when opening in browser")
		}

		trendings, err := GetTrending(func() string {
			if len(args) == 1 {
				return args[0]
			}
			return ""
		}())
		if err != nil {
			return err
		}

		if *openInBrowser {
			b := browser.New("", os.Stdout, os.Stderr)
			wg := sync.WaitGroup{}
			for _, trending := range trendings {
				wg.Add(1)
				go func(u string) {
					defer wg.Done()
					if err := b.Browse(u); err != nil {
						panic(err)
					}
				}(trending.GetHTMLURL())
			}
			wg.Wait()

			return nil
		}

		switch outputFormat(*outFormat) {
		case JSON:
			var b bytes.Buffer
			trendingJSONbuf := bufio.NewReadWriter(
				bufio.NewReader(&b), bufio.NewWriter(&b),
			)
			if err := json.NewEncoder(trendingJSONbuf).Encode(trendings); err != nil {
				return err
			}

			if err := jsonpretty.Format(
				io.Writer(os.Stdout), io.Reader(trendingJSONbuf), " ", true,
			); err != nil {
				return err
			}
		case table:
			width, _, err := term.FromEnv().Size()
			if err != nil {
				return err
			}

			tp := tableprinter.New(os.Stdout, term.IsTerminal(os.Stdout), width)
			for _, header := range []string{
				"OWNER", "NAME", "LANG", "URL", "STARS", "DESC",
			} {
				tp.AddField(header)
			}
			tp.EndRow()

			for _, trending := range trendings {
				tp.AddField(*trending.Owner.Login)
				tp.AddField(*trending.Name)
				tp.AddField(func() string {
					if trending.Language != nil {
						return *trending.Language
					}
					return ""
				}())
				tp.AddField(trending.GetHTMLURL())
				tp.AddField(fmt.Sprint(*trending.StargazersCount))
				tp.AddField(func() string {
					if trending.Description != nil {
						return *trending.Description
					}
					return ""
				}())
				tp.EndRow()
			}

			return tp.Render()
		}

		return nil
	},
}

func init() {
	log.SetOutput(os.Stderr)
	openInBrowser = trendingCmd.PersistentFlags().BoolP(
		"web", "w", false, "Open in web browser",
	)
	outFormat = trendingCmd.PersistentFlags().StringP(
		"output", "o", string(table), "Output format (valid: json, table)",
	)
}

func main() {
	if err := trendingCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
