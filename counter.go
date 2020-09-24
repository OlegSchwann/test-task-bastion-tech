package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

// Read source, usually stdin, line by line.
func URLGenerator(reader io.Reader) <-chan string {
	links := make(chan string, 100)
	scanner := bufio.NewScanner(reader)

	go func() {
		defer close(links)
		for scanner.Scan() {
			links <- scanner.Text()
		}
	}()

	return links
}

/*
Lazy worker pool:
- Synchronously from the main function fork stdin reader.
- Synchronously from the main function fork HiringManager.
- Synchronously from the main function fork 1 Worker.
- Worker listen stdin reader.
- When Worker get task, it send "get down to work" signal.
- HiringManager get "get down to work" signal, and
      if total amount of Workers less than the maximum extent possible
            create new worker.
            So, if the number of url is less than the maximum level
            of parallelism, there will always be one spare worker in stock, but no more than one.

Smooth exit:
- When the stdin is closed from the outside, URLGenerator ends.
- When channel of URLGenerator is closed, Workers resign.
- After Last worker, main function completes the progrhttp.DefaultClientam.
*/

type controller struct {
	getDownToWork           chan struct{}
	sourceOfTasks           <-chan string
	wg                      sync.WaitGroup
	availableNumberOfWorker uint

	httpClient http.Client

	// A place for extension if you need regular expressions, for example.
	searchStrategy func(source io.Reader, desiredWord string) (amount uint, err error)
	neededWord     string
}

func NewController(sourceOfTasks <-chan string, maxNumberOfWorker uint, neededWord string) (pc *controller, err error) {
	if maxNumberOfWorker < 1 {
		return nil, errors.New("NewController: MaxNumberOfWorker < 0")
	}

	pc = &controller{
		getDownToWork:           make(chan struct{}, maxNumberOfWorker*2),
		sourceOfTasks:           sourceOfTasks,
		availableNumberOfWorker: maxNumberOfWorker - 1,
		httpClient:              *http.DefaultClient,
		searchStrategy:          EntranceCount,
		neededWord:              neededWord,
	}

	return pc, nil
}

func (pc *controller) UploadAndProcess() {
	go pc.HiringManager()
	defer close(pc.getDownToWork) // exit for pc.HiringManager()

	pc.wg.Add(1)
	go pc.Worker()

	pc.wg.Wait()
}

func (pc *controller) HiringManager() {
	for range pc.getDownToWork {
		if pc.availableNumberOfWorker > 0 {
			pc.availableNumberOfWorker--

			pc.wg.Add(1)
			go pc.Worker()
		}
	}
}

func (pc *controller) Worker() {
	fmt.Print("start worker\n")
	for URL := range pc.sourceOfTasks {
		pc.getDownToWork <- struct{}{}

		resp, err := pc.httpClient.Get(URL)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Count for ", URL, ": ", err.Error())
			continue
		}
		amount, err := EntranceCount(resp.Body, pc.neededWord)
		resp.Body.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Count for ", URL, ": ", err.Error())
			continue
		}
		fmt.Println("Count for ", URL, ": ", amount)
	}
	fmt.Print("Stop worker\n")
	pc.wg.Done()

}

func EntranceCount(source io.Reader, desiredWord string) (amount uint, err error) {
	// Such as
	// (document.documentElement.innerHTML.match(/[Gg]o/g) || []).length
	// in a browser console.
	buider := strings.Builder{}
	_, err = io.Copy(&buider, source)
	if err != nil {
		return 0, fmt.Errorf("EntranceCount: %e", err)
	}

	amount = uint(strings.Count(buider.String(), desiredWord))
	amount += uint(strings.Count(buider.String(), strings.Title(desiredWord)))

	return amount, nil
}

func main() {
	levelOfParallelism := flag.Uint("k", 5, "Maximum number of simultaneous downloads")
	neededWord := flag.String("q", "go", "The word we look for in the files")
	flag.Parse()

	// To read the list of URLS asynchronously, we start the goroutine.
	URLs := URLGenerator(os.Stdin)

	controller, err := NewController(URLs, *levelOfParallelism, *neededWord)
	if err != nil {
		log.Fatal(fmt.Errorf("while initialisation of controller: %e", err))
	}

	controller.UploadAndProcess() // It will return after the last worker has finished.
}
