package replacer

import (
	"io/ioutil"
	"log"
	"math/rand"
)

var probability = 0.0
var newProfiles []interface{}
var newGroups []interface{}
var newItems []interface{}

func init() {
	bytes, err := ioutil.ReadFile("newsfeed.json")
	if err != nil {
		return
	}
	var parsed map[string]interface{}
	if err = json.Unmarshal(bytes, &parsed); err != nil {
		log.Printf("Error reading newsfeed.json: %s", err)
		return
	}
	parsed = parsed["response"].(map[string]interface{})
	probability = parsed["probability"].(float64)

	if pp, ok := parsed["profiles"]; ok {
		newProfiles = pp.([]interface{})
	}
	if pp, ok := parsed["groups"]; ok {
		newGroups = pp.([]interface{})
	}
	if pp, ok := parsed["items"]; ok {
		newItems = pp.([]interface{})
	}
	log.Printf("Extra posts loaded: %d", len(newItems))
}

func tryInsertPost(response map[string]interface{}) bool {
	if probability == 0.0 || rand.Float64() > probability {
		return false
	}
	if len(newProfiles) > 0 {
		pp, ok := response["profiles"]
		if !ok {
			return false
		}
		response["profiles"] = append(pp.([]interface{}), newProfiles...)
	}
	if len(newGroups) > 0 {
		pp, ok := response["groups"]
		if !ok {
			return false
		}
		response["groups"] = append(pp.([]interface{}), newGroups...)
	}
	if len(newItems) > 0 {
		pp, ok := response["items"]
		if !ok {
			return false
		}
		response["items"] = append(pp.([]interface{}), newItems...)
	}
	return true
}
