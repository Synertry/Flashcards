// Stage 7/7 of Flashcards (Go): https://hyperskill.org/projects/224/stages/1127/implement
// Functions and Methods are sorted in call order

package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type (
	/*
		Flashcards.deck is a 2-key map, "term" or "definition", which holds specific terms or definitions.
		One card results in two entries, example:
		Flashcards.Deck["term"]["question"] == "answer"
		Flashcards.Deck["definition"]["answer"] == "question"

		Flashcards.Stats field holds statistics on terms
		Example with mistakes:
		Flashcards.Stats["mistakes"]["question"] == 0

		logger holds the TeeWriter to simultaneously write to os.Stdout and log

		params stores available flags
	*/
	Flashcards struct {
		Deck   map[string]map[string]string
		Stats  map[string]map[string]int
		log    bytes.Buffer
		logger io.Writer
		params map[string]*string
	}

	// termMistakes is a helper struct for slice sorting in Flashcards.HardestCard
	termMistakes struct {
		term     string
		mistakes int
	}
)

func main() {
	cards := new(Flashcards)
	cards.Action()
}

// Action decides on user input which method of Flashcards to invoke
func (cards *Flashcards) Action() {
	cards.init()
	for {
		log.Println("Input the action (add, remove, import, export, ask, exit, log, hardest card, reset stats):")

		switch cards.getInput() {
		case "add":
			cards.Add()
		case "remove":
			cards.Remove()
		case "import":
			cards.Import()
		case "export":
			cards.Export()
		case "ask":
			cards.Ask()
		case "exit":
			cards.Exit()
		case "log":
			cards.Log()
		case "hardest card":
			cards.HardestCard()
		case "reset stats":
			cards.ResetStats()
		default:
			log.Printf("No valid input\n\n") // custom, not task relevant
		}
	}
}

// init pre-allocates memory for the maps, sets up a tee logger and parse command flags
func (cards *Flashcards) init() {
	cards.Deck = make(map[string]map[string]string, 2)
	cards.Deck["term"] = map[string]string{}
	cards.Deck["definition"] = map[string]string{}
	cards.Stats = map[string]map[string]int{}
	cards.Stats["mistakes"] = map[string]int{}
	cards.logger = io.MultiWriter(os.Stdout, &cards.log)
	log.SetOutput(cards.logger) // it's more convenient not to consistently check error, log.Print does not return errors, like fmt.Fprint does
	log.SetFlags(0)             // log.Print still outputs a newline though
	cards.params = make(map[string]*string, 2)
	cards.params["importFrom"] = flag.String("import_from", "", "file to import a deck from")
	cards.params["exportTo"] = flag.String("export_to", "", "file to export to at exit")
	flag.Parse()
	if strings.TrimSpace(*cards.params["importFrom"]) != "" {
		cards.Import()
	} // param import_from
}

// getInput reads from os.Stdin, write it to cards.log buffer and returns a string trimmed from spaces and newline chars
func (cards *Flashcards) getInput() string { // this makes no sense to use it as a func, but I needed an easy way to access Flashcards.log buffer, this harms readability
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadBytes('\n')
	cards.log.Write(input)
	errCheck(err)
	return strings.Trim(string(input), " \r\n")
}

// errCheck is an error checker, which panics on errors; it is the most referenced func
func errCheck(err error) {
	if err != nil {
		panic(err.Error())
	}
}

// Add is invoked by Action, adds a term and a definition to Flashcards.deck
func (cards *Flashcards) Add() {
	var term, definition string
	log.Println("The card:")
	for term == "" {
		term = cards.addEntry("term", cards.getInput())
	}
	log.Println("The definition of the card:")
	for definition == "" {
		definition = cards.addEntry("definition", cards.getInput())
	}

	cards.Deck["term"][term] = definition
	cards.Deck["definition"][definition] = term
	cards.Stats["mistakes"][term] = 0 // we don't actually need this, incrementing inits it and reset sets it to 0 anyway
	log.Printf("The pair (\"%s\":\"%s\") has been added.\n\n", term, definition)
}

// addEntry adds either a term or a definition to Flashcard.Deck, depending on the group parameter and return non-empty string if successful
func (cards *Flashcards) addEntry(group, entry string) string {
	if entry == "" { // prevent empty entry as key, the control flow depends on empty strings
		log.Printf("Empty %ss are not allowed. Try again:\n", group) // custom, not task relevant
		return ""
	}
	if _, ok := cards.Deck[group][entry]; ok {
		if group == "term" {
			group = "card"
		}
		log.Printf("The %s \"%s\" already exists. Try again:\n", group, entry)
		return ""
	}
	cards.Deck[group][entry] = ""
	return entry
}

// Remove discards an existing entry in both sub maps
func (cards *Flashcards) Remove() {
	log.Println("Which card?")

	term := cards.getInput()
	if definition, ok := cards.Deck["term"][term]; ok {
		delete(cards.Deck["term"], term)
		delete(cards.Deck["definition"], definition)
		delete(cards.Stats["mistakes"], term)
		log.Println("The card has been removed.")
	} else {
		log.Printf("Can't remove \"%s\": there is no such card.\n", term)
	}
	log.Println()
}

// Import opens specified file if it exits, invokes importParse for every line read and assigns entries to deck
func (cards *Flashcards) Import() {
	mode := "Read"
	if strings.TrimSpace(*cards.params["importFrom"]) != "" {
		mode = "Import"
	}

	if file, err, fclose := cards.getFile(mode); errors.Is(err, os.ErrNotExist) {
		log.Printf("File not found.\n\n")
	} else {
		errCheck(err)
		defer fclose(file)

		var t, d string // term, definition
		var m, i int    // mistakes, counter
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if scannerErr := scanner.Err(); scannerErr != nil {
				if errors.Is(scannerErr, io.EOF) {
					break
				}
				errCheck(scannerErr)
			}
			t, d, m = cards.importParse(scanner.Text())
			cards.Deck["term"][t] = d
			cards.Deck["definition"][d] = t
			cards.Stats["mistakes"][t] = m
			i++
		}
		log.Printf("%d cards have been loaded.\n\n", i)
		errCheck(scanner.Err())
	}
}

// getFile return *os.File with specified mode, err to check, specifically io.EOF and close func to defer
func (cards *Flashcards) getFile(mode string) (file *os.File, err error, fclose func(file *os.File)) {
	var fileName string

	switch mode { // cmdline params check
	case "Import":
		mode = "Read"
		fileName = *cards.params["importFrom"]
		*cards.params["importFrom"] = ""
	case "Export":
		mode = "Write"
		fileName = *cards.params["exportTo"]
		*cards.params["exportTo"] = ""
	default:
		log.Println("File name:")
		fileName = cards.getInput()
	}

	switch mode {
	case "Read":
		file, err = os.Open(fileName)
	case "Write":
		file, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
	}

	return file, err, func(*os.File) {
		errCheck(file.Close())
	}
}

// importParse splits the valid string from Import into term, definition and mistakes; panics if line is not in format ("term":"definition"):(mistakes)
func (cards *Flashcards) importParse(line string) (term, definition string, mistakes int) {
	parser, err := regexp.Compile(`^\("([^"]+)":"([^"]+)"\):\((\d+)\)$`) // Golang does not support lookarounds
	errCheck(err)
	mistakes, err = strconv.Atoi(parser.FindStringSubmatch(line)[3])
	errCheck(err)
	return parser.FindStringSubmatch(line)[1], parser.FindStringSubmatch(line)[2], mistakes
}

// Export writes all term:definition:mistakes groups to specified file, any existing file will be overwritten
func (cards *Flashcards) Export() {
	mode := "Write"
	if strings.TrimSpace(*cards.params["exportTo"]) != "" && len(cards.params) == 1 {
		mode = "Export"
	}

	file, err, fclose := cards.getFile(mode)
	errCheck(err)
	defer fclose(file)

	i := 0
	for t, d := range cards.Deck["term"] {
		_, err = fmt.Fprintf(file, "(\"%s\":\"%s\"):(%d)\n", t, d, cards.Stats["mistakes"][t])
		errCheck(err)
		i++
	}
	if mode == "Export" {
		fmt.Printf("\n\n%d cards have been saved.", i)
	} else {
		log.Printf("%d cards have been saved.\n\n", i)
	}
}

// Ask gets input n from user and invokes askEntry n times
func (cards *Flashcards) Ask() {
	log.Println("How many times to ask?")
	n, err := strconv.Atoi(cards.getInput()) // maybe add limit for n
	if err != nil {
		panic(err)
	}

	orderedTerms := cards.getOrderedTerms()
	for i := 0; i < n; i++ {
		cards.askEntry(orderedTerms)
	}
	log.Println()
}

// getOrderedTerms returns an indexed slice with all currently loaded terms
func (cards *Flashcards) getOrderedTerms() *[]string {
	orderedTerms := make([]string, len(cards.Deck["term"]))
	i := 0
	for term := range cards.Deck["term"] {
		orderedTerms[i] = term
		i++
	}
	return &orderedTerms
}

// askEntry asks the user a random term from Flashcards.Deck
func (cards *Flashcards) askEntry(orderedTerms *[]string) {
	getRandomNum := func(min, max int) int { // randomizer
		rand.Seed(time.Now().UnixNano())
		return rand.Intn(max-min+1) + min
	}

	definition := cards.Deck["term"][(*orderedTerms)[getRandomNum(0, len(*orderedTerms)-1)]]
	term := cards.Deck["definition"][definition]

	log.Printf("Print the definition of \"%s\":\n", term)

	answer := cards.getInput()
	if answer == definition {
		log.Println("Correct!")
	} else {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Wrong. The right answer is \"%s\"", definition))
		if val, ok := cards.Deck["definition"][answer]; ok {
			sb.WriteString(fmt.Sprintf(", but your definition is correct for \"%s\"", val))
		}
		sb.WriteString(".")
		log.Println(sb.String())
		cards.Stats["mistakes"][term]++ // would init map entry automatically, but we already did it manually
	}
}

// Exit farewells the user and exports deck if parameter has been provided
func (cards *Flashcards) Exit() {
	fmt.Print("Bye bye!")
	delete(cards.params, "importFrom")
	if strings.TrimSpace(*cards.params["exportTo"]) != "" {
		cards.Export()
	} // check param export_to
	os.Exit(0)
}

// Log gets input from user as path string and writes Flashcards.log to a file
func (cards *Flashcards) Log() {
	log.Println("File name:")
	errCheck(os.WriteFile(cards.getInput(), cards.log.Bytes(), 0644)) // overwrites
	log.Printf("The log has been saved.\n\n")
}

// HardestCard prints the cards(s) with the highest int in Flashcards.Stats.["mistakes"]
func (cards *Flashcards) HardestCard() {
	sS := cards.createStatsSlice("mistakes")
	if maxM := sS[len(sS)-1].mistakes; maxM > 0 {
		hardCards := cards.findHardestCard(sS, maxM, len(sS))
		cards.printHardestCard(hardCards, maxM)
	} else {
		log.Println("There are no cards with errors.")
	}
	log.Println()
}

// createStatsSlice returns sorted statsSlice for stat-type, e.g. "mistakes"
func (cards *Flashcards) createStatsSlice(stat string) []*termMistakes {
	if len(cards.Stats[stat]) == 0 { // in case there are no cards loaded
		return []*termMistakes{{term: "", mistakes: 0}}
	}

	sS := make([]*termMistakes, 0, len(cards.Stats[stat])) // sS: statsSlice
	for t, m := range cards.Stats[stat] {
		sS = append(sS, &termMistakes{term: t, mistakes: m})
	}
	sort.SliceStable(sS, func(i, j int) bool {
		return sS[i].mistakes < sS[j].mistakes
	})
	return sS
}

// findHardestCard returns slice of the hardest card(s), pos is needed because of the lookup in reversed order
func (cards *Flashcards) findHardestCard(sS []*termMistakes, maxM int, pos int) []string {
	hardCards := make([]string, 0, len(sS))
	for i := 0; i < len(sS); i++ {
		pos--
		if sS[pos].mistakes != maxM {
			break
		}
		hardCards = append(hardCards, sS[pos].term)
	}
	return hardCards
}

// printHardestCard outputs the slice of hardCards
func (cards *Flashcards) printHardestCard(hardCards []string, mCount int) {
	var sb strings.Builder
	if len(hardCards) == 1 {
		sb.WriteString(fmt.Sprintf("The hardest card is \"%s\". You have %d errors answering it.", hardCards[0], mCount))
	} else {
		sb.WriteString(fmt.Sprint("The hardest cards are "))
		for i := range hardCards {
			if i != 0 {
				sb.WriteString(fmt.Sprint(", "))
			}
			sb.WriteString(fmt.Sprintf("\"%s\"", hardCards[i]))
		}
		sb.WriteString(fmt.Sprintf(". You have %d errors answering them.", mCount))
	}
	log.Print(sb.String())
}

// ResetStats sets all int values in Flashcards.Stats to 0
func (cards *Flashcards) ResetStats() {
	for term := range cards.Stats["mistakes"] {
		cards.Stats["mistakes"][term] = 0
	}
	log.Printf("Card statistics have been reset.\n\n")
}
