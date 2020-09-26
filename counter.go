package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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
		if err := scanner.Err(); err != nil {
			fmt.Fprint(os.Stderr, "URLGenerator: ", err)
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
	workerWG                sync.WaitGroup
	staffWG                 sync.WaitGroup
	availableNumberOfWorker uint
	statistics              chan statistics

	httpClient http.Client

	// A place for extension if you need regular expressions, for example.
	searchStrategy func(source io.Reader, desiredWord string) (amount uint, err error)
	neededWord     string
}

type statistics struct {
	totalCount uint
}

func NewController(sourceOfTasks <-chan string, maxNumberOfWorker uint, neededWord string) (pc *controller, err error) {
	if maxNumberOfWorker < 1 {
		return nil, errors.New("NewController: MaxNumberOfWorker < 0")
	}

	pc = &controller{
		getDownToWork:           make(chan struct{}, maxNumberOfWorker+1),
		sourceOfTasks:           sourceOfTasks,
		availableNumberOfWorker: maxNumberOfWorker - 1,
		statistics:              make(chan statistics, maxNumberOfWorker+1),
		httpClient:              http.Client{Timeout: 10 * time.Second},
		searchStrategy:          StreamEntranceCount,
		neededWord:              neededWord,
	}

	return pc, nil
}

func (pc *controller) UploadAndProcess() {
	pc.staffWG.Add(1)
	go pc.HiringManager()

	pc.staffWG.Add(1)
	go pc.Analyst()

	pc.workerWG.Add(1)
	go pc.Worker()

	pc.workerWG.Wait()

	close(pc.getDownToWork) // exit for pc.HiringManager()
	close(pc.statistics)    // exit for pc.Analyst()
	pc.staffWG.Wait()
}

func (pc *controller) HiringManager() {
	defer pc.staffWG.Done()

	for range pc.getDownToWork {
		if pc.availableNumberOfWorker > 0 {
			pc.availableNumberOfWorker--

			pc.workerWG.Add(1)
			go pc.Worker()
		}
	}
}

func (pc *controller) Analyst() {
	defer pc.staffWG.Done()

	var count uint
	for stat := range pc.statistics {
		count += stat.totalCount
	}
	fmt.Println("Total: ", count)
}

func (pc *controller) Worker() {
	defer pc.workerWG.Done()
	for URL := range pc.sourceOfTasks {
		pc.getDownToWork <- struct{}{}
		func() {
			resp, err := pc.httpClient.Get(URL)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Count for ", URL, ": ", err.Error())
				return
			}
			defer resp.Body.Close()

			amount, err := pc.searchStrategy(resp.Body, pc.neededWord)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Count for ", URL, ": ", err.Error())
				return
			}
			pc.statistics <- statistics{
				totalCount: amount,
			}

			fmt.Println("Count for ", URL, ": ", amount)
		}()
	}
}

var StreamSearcherBuffSize = 32 * 1024 // io.Copy() use 32 kb.

func StreamEntranceCount(source io.Reader, desiredWord string) (amount uint, err error) {
	desiredBytes := []byte(desiredWord)
	desiredBytesTitle := []byte(strings.Title(desiredWord))

	// Add len of desiredBytes, if searched bytes are slitted between result of two .Read() calls.
	buf := make([]byte, StreamSearcherBuffSize+len(desiredBytes)-1)

	// Each time the data tail from the previous iteration is copied to the beginning of the buffer.
	// If the searched word is divided between buffers, it will be found.
	for {
		n, err := source.Read(buf[len(desiredBytes)-1:])
		if n > 0 {
			amount += uint(bytes.Count(buf[:len(desiredBytes)-1+n], desiredBytes))
			amount += uint(bytes.Count(buf[:len(desiredBytes)-1+n], desiredBytesTitle))
		}
		if err != nil {
			if err == io.EOF {
				return amount, nil
			}
			return amount, fmt.Errorf("StreamEntranceCount: %e", err)
		}
		copy(buf[:len(desiredBytes)-1], buf[n:])
	}
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
