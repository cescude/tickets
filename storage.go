package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Username   string
	Token      string
	Root       string
	LastUpdate time.Time
}

type Ticket struct {
	Key         string
	Title       string
	NumComments int
	Assignee    string
	Status      string
	Hash        string
	New         bool
}

type DataFile struct {
	Cfg     Config
	Tickets []Ticket
}

var Username string
var Token string
var JiraRoot string
var Tickets []Ticket
var LastUpdate time.Time

var defaultJson string = `{
  "Cfg": { 
    "Root": "https://readingplus.atlassian.net",
    "Username": "<YOUR_NAME>@readingplus.com", 
    "Token": "<API_TOKEN>"
  }
}`

func settingsFile() string {
	homedir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	return filepath.Join(homedir, ".tickets.conf")
}

// Returns true if loaded successfully, false otherwise
func Load() {
	filename := settingsFile()

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("%s\nPlease create a JSON file %s with the following contents:\n%s",
			err,
			filename,
			defaultJson)
	}

	var df DataFile
	err = json.Unmarshal(data, &df)
	if err != nil {
		log.Fatal(err)
	}

	JiraRoot = df.Cfg.Root
	Username = df.Cfg.Username
	Token = df.Cfg.Token
	Tickets = df.Tickets
	LastUpdate = df.Cfg.LastUpdate
}

func Save(tickets []Ticket) error {
	filename := settingsFile()

	df := DataFile{}
	df.Cfg = Config{Username: Username, Token: Token, Root: JiraRoot, LastUpdate: time.Now()}
	df.Tickets = tickets

	data, err := json.Marshal(df)
	if err != nil {
		log.Fatal(err)
	}

	return ioutil.WriteFile(filename, data, 0600)
}
