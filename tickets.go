package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bbrks/wrap"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func failif(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func encode(s string) string {
	data := []byte(s)
	return base64.StdEncoding.EncodeToString(data)
}

func auth(username, apikey string) string {
	return fmt.Sprintf("Basic %s", encode(fmt.Sprintf("%s:%s", username, apikey)))
}

func jiraTicketLink(key string) string {
	return fmt.Sprintf("%s/rest/api/latest/issue/%s", JiraRoot, key)
}

func jiraBrowseUrl(key string) string {
	return fmt.Sprintf("%s/browse/%s", JiraRoot, key)
}

func searchLink(jql, fields string, start_at int) string {
	return fmt.Sprintf("%s/rest/api/latest/search?maxResults=100&jql=%s&fields=%s&startAt=%d",
		JiraRoot,
		jql,
		fields,
		start_at*100)
}

type searchResults struct {
	Issues []JsonIssue
}

type JsonIssue struct {
	Key    string
	Fields struct {
		Summary     string
		Description string
		Comment     struct {
			Comments []struct {
				Author struct {
					DisplayName string
				}
				Body    string
				Updated string
			}
		}
		Assignee struct {
			Key         string
			DisplayName string
		}
		Status struct {
			Name string
		}
	}
}

func search(jql, fields string, start_at int) searchResults {
	req, err := http.NewRequest("GET", searchLink(jql, fields, start_at), nil)
	failif(err)

	req.Header.Set("Authorization", auth(Username, Token))
	req.Header.Set("X-Atlassian-Token", "no-check")

	client := &http.Client{}
	resp, err := client.Do(req)
	failif(err)
	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	failif(err)

	var dat searchResults
	err = json.Unmarshal(bytes, &dat)
	failif(err)

	// dump(bytes)

	return dat
}

func dump(b []byte) {
	var out bytes.Buffer
	json.Indent(&out, b, "", " ")
	out.WriteTo(os.Stdout)
}

func fingerprint(issue JsonIssue) string {
	hash := sha256.Sum256([]byte(
		fmt.Sprintf("%s\n%s\n%d\n%s",
			issue.Key,
			issue.Fields.Summary,
			len(issue.Fields.Comment.Comments),
			issue.Fields.Description)))
	return fmt.Sprintf("%x", hash)
}

func find(ticket Ticket, tickets []Ticket) (bool, Ticket) {
	for _, t := range tickets {
		if ticket.Key == t.Key {
			return true, t
		}
	}
	return false, Ticket{}
}

// Given two lists of Tickets, find the "common" tickets.  If the hash is
// different, keep the left side, otherwise keep the right.  Returns both lists
// combined, with new-hash tickets at the top.
func unifyLists(newList, oldList []Ticket) []Ticket {
	results := make([]Ticket, 0)

	for _, n := range newList {
		found, o := find(n, oldList)
		shouldAppend := !found || n.Hash != o.Hash

		if shouldAppend {
			n.New = true
			results = append(results, n)
		}
	}

	for _, o := range oldList {
		found, n := find(o, newList)
		shouldAppend := !found || o.Hash == n.Hash

		if shouldAppend {
			// If the field was found in the newlist, but the hash
			// hasn't changed, still update some useful fields
			// (e.g. "Status" isn't something that hash takes into
			// account, but we do want it to be current)
			if found {
				o.Status = n.Status
				o.Assignee = n.Assignee
			}

			if Conf.Clear {
				o.New = false
			}

			results = append(results, o)
		}
	}

	return results
}

func loadRecentTickets(terminalHashes map[string]bool) []Ticket {
	foundTickets := make([]Ticket, 0)

	for pageNumber := 0; pageNumber < 15; pageNumber++ {
		jira := search(
			"ORDER+BY+updated+DESC",
			"summary,comment,description,updated,assignee,reporter,status",
			pageNumber)

		foundTerminalHash := len(jira.Issues) == 0 // If no issues were returned, we're done

		for i, _ := range jira.Issues {
			issue := jira.Issues[i]
			t := Ticket{}
			t.Key = issue.Key
			t.Title = issue.Fields.Summary
			t.NumComments = len(issue.Fields.Comment.Comments)
			t.Assignee = issue.Fields.Assignee.DisplayName
			t.Status = issue.Fields.Status.Name
			t.Hash = fingerprint(issue)

			if _, present := terminalHashes[t.Hash]; present {
				foundTerminalHash = true
			}

			foundTickets = append(foundTickets, t)
		}

		if foundTerminalHash {
			break
		}
	}

	return foundTickets
}

func list(clearRead, showAll, refresh bool) {
	terminalHashes := make(map[string]bool)
	for _, t := range Tickets {
		terminalHashes[t.Hash] = true
	}

	foundTickets := make([]Ticket, 0)
	if refresh {
		foundTickets = loadRecentTickets(terminalHashes)
	}

	unified := unifyLists(foundTickets, Tickets)

	if clearRead || refresh {
		err := Save(unified)
		failif(err)
	}

	for _, t := range unified {
		if t.New || showAll {
			var star string
			if showAll && t.New {
				star = "*"
			} else {
				star = ""
			}
			fmt.Printf("%-10s %1s %2d %s (%s, %s)\n",
				t.Key,
				star,
				t.NumComments,
				t.Title,
				t.Status,
				strings.Split(t.Assignee, " ")[0])
		} else {
			break
		}
	}
}

func show(key string) {
	req, err := http.NewRequest("GET", jiraTicketLink(key), nil)
	failif(err)

	req.Header.Set("Authorization", auth(Username, Token))
	req.Header.Set("X-Atlassian-Token", "no-check")

	client := &http.Client{}
	resp, err := client.Do(req)
	failif(err)
	defer resp.Body.Close()

	bytes, err := ioutil.ReadAll(resp.Body)
	failif(err)

	var issue JsonIssue
	err = json.Unmarshal(bytes, &issue)
	failif(err)

	// dump(bytes)

	separator := "~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"

	fmt.Printf("%s %s\n%s\n%s\n%s, %s\n\n%s\n",
		issue.Key,
		issue.Fields.Summary,
		jiraBrowseUrl(issue.Key),
		separator,
		issue.Fields.Status.Name,
		issue.Fields.Assignee.DisplayName,
		strings.TrimSpace(wrap.Wrap(issue.Fields.Description, 80)))

	for _, c := range issue.Fields.Comment.Comments {
		fmt.Printf("%s\n%s\n%s\n\n%s\n",
			separator,
			c.Author.DisplayName,
			c.Updated,
			strings.TrimSpace(wrap.Wrap(c.Body, 80)))
	}
}

var Conf struct {
	Clear bool
	All   bool
}

func main() {
	flag.BoolVar(&Conf.Clear, "clear", false, "Marks all old tickets as read")
	flag.BoolVar(&Conf.All, "all", false, "Display all Jira tickets, not just unread")
	flag.Parse()

	Load()

	tickets := flag.Args()

	switch {
	case len(tickets) > 0:
		for _, ticket := range tickets {
			show(ticket)
			fmt.Println("")
		}
	default:
		refresh := time.Since(LastUpdate).Minutes() > 1.0
		list(Conf.Clear, Conf.All, refresh)
	}
}
