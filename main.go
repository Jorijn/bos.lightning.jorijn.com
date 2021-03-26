package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"
)

type OneMLAddress struct {
	Network string `json:"network"`
	Address string `json:"addr"`
}

type OneMLNodeRank struct {
	Capacity     uint `json:"capacity"`
	ChannelCount uint `json:"channelcount"`
	Age          uint `json:"age"`
	Growth       uint `json:"growth"`
	Availability uint `json:"availability"`
}

type OneMLData struct {
	LastUpdate   uint           `json:"last_update"`
	PublicKey    string         `json:"pub_key"`
	Alias        string         `json:"alias"`
	Addresses    []OneMLAddress `json:"addresses"`
	Color        string         `json:"color"`
	Capacity     uint           `json:"capacity"`
	ChannelCount uint           `json:"channelcount"`
	NodeRank     OneMLNodeRank  `json:"noderank"`
}

type Score struct {
	Alias     string `json:"alias"`
	PublicKey string `json:"public_key"`
	Score     uint   `json:"score"`
}

type EnrichedScore struct {
	Score            uint     `json:"score"`
	Alias            string   `json:"alias"`
	PublicKey        string   `json:"publicKey"`
	Addresses        []string `json:"addresses"`
	Color            string   `json:"color"`
	Capacity         uint     `json:"capacity"`
	ChannelCount     uint     `json:"channelCount"`
	RankCapacity     uint     `json:"rankCapacity"`
	RankChannelCount uint     `json:"rankChannelCount"`
	RankAge          uint     `json:"rankAge"`
	RankGrowth       uint     `json:"rankGrowth"`
	RankAvailability uint     `json:"rankAvailability"`
}

type EnrichedList struct {
	LastUpdated string          `json:"lastUpdated"`
	Data        []EnrichedScore `json:"data"`
}

type BosList struct {
	LastUpdated string  `json:"last_updated"`
	Scores      []Score `json:"scores"`
}


func worker(workerId int, bosScores chan Score, waitGroup *sync.WaitGroup, enrichedScores chan EnrichedScore) {
	defer waitGroup.Done()

	for {
		bosScore, ok := <-bosScores

		if !ok {
			log.Printf("closing worker %d", workerId)
			return
		}

		log.Println("worker", workerId, "fetching", bosScore.Alias)

		nodeBytes, err := downloadFromAPI("https://1ml.com/node/" + bosScore.PublicKey + "/json")
		if err != nil {
			log.Printf("ERROR: could not download data for %s: %s", bosScore.PublicKey, err)
			continue
		}

		oneMLData := OneMLData{}
		err = json.Unmarshal(nodeBytes, &oneMLData)
		if err != nil {
			log.Printf("ERROR: could not parse data for %s: %s (%s)", bosScore.PublicKey, err.Error(), string(nodeBytes))
			continue
		}

		var addresses []string
		for _, address := range oneMLData.Addresses {
			addresses = append(addresses, address.Address)
		}

		// re-map the data as we see fit
		enrichedScore := EnrichedScore{
			Score:            bosScore.Score,
			Alias:            bosScore.Alias,
			PublicKey:        bosScore.PublicKey,
			Addresses:        addresses,
			Color:            oneMLData.Color,
			Capacity:         oneMLData.Capacity,
			ChannelCount:     oneMLData.ChannelCount,
			RankCapacity:     oneMLData.NodeRank.Capacity,
			RankChannelCount: oneMLData.NodeRank.ChannelCount,
			RankAge:          oneMLData.NodeRank.Age,
			RankGrowth:       oneMLData.NodeRank.Growth,
			RankAvailability: oneMLData.NodeRank.Availability,
		}

		// push it back to the coordinating routine
		enrichedScores <- enrichedScore
	}
}

func main() {
	jsonBytes, err := downloadFromAPI("https://nodes.lightning.computer/availability/v1/btc.json")
	if err != nil {
		log.Fatal(err)
	}

	bosList := BosList{}

	err = json.Unmarshal(jsonBytes, &bosList)
	if err != nil {
		log.Fatalf("unable to parse value: %s, error: %s", string(jsonBytes), err.Error())
	}

	var wg sync.WaitGroup

	queueScores := make(chan Score)
	queueEnrichedScores := make(chan EnrichedScore, len(bosList.Scores))
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go worker(i, queueScores, &wg, queueEnrichedScores)
	}

	for _, bosScore := range bosList.Scores {
		queueScores <- bosScore
	}

	// after we're done, close the queue so the worker threads stop too
	close(queueScores)

	// wait for all workers to finish, then close the resulting queue with results
	// https://gobyexample.com/range-over-channels
	wg.Wait()
	close(queueEnrichedScores)

	// gather the enriched scores
	var enrichedScores []EnrichedScore
	for enrichedScore := range queueEnrichedScores {
		enrichedScores = append(enrichedScores, enrichedScore)
	}

	now := time.Now()
	enrichedList := EnrichedList{
		LastUpdated: now.Format("2006-01-02T15:04:05-0700"),
		Data:        enrichedScores,
	}

	buf, err := json.MarshalIndent(enrichedList, "", "  ")
	if err != nil {
		log.Fatal("could not write enriched list to JSON", err)
	}

	err = ioutil.WriteFile("web/data/export.json", buf, 0644)
	if err != nil {
		log.Fatal("could not write JSON file to file", err)
	}

	log.Printf("written %d bytes to web/data/export.json\n", len(buf))
}

func downloadFromAPI(url string) ([]byte, error) {
	downloadClient := http.Client{
		Timeout: time.Second * 2,
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	log.Printf("downloading from url %s", url)

	res, err := downloadClient.Do(req)
	if err != nil {
		return nil, err
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	return body, nil
}
