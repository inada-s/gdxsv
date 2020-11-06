package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var retryTimer *time.Timer
var jobIsRunning uint32

type onlineUser struct {
	UserID     string `json:"user_id,omitempty"`
	Name       string `json:"name,omitempty"`
	Team       string `json:"team,omitempty"`
	BattleCode string `json:"battle_code,omitempty"`
	Disk       string `json:"disk,omitempty"`
}

type discordEmbedFooter struct {
	Text string `json:"text,omitempty"`
}

type discordEmbedField struct {
	Name   string `json:"name,omitempty"`
	Value  string `json:"value,omitempty"`
	Inline bool   `json:"inline,omitempty"`
}

type discordEmbed struct {
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Color       int                  `json:"color,omitempty"`
	Footer      *discordEmbedFooter  `json:"footer,omitempty"`
	Timestamp   string               `json:"timestamp,omitempty"`
	Fields      []*discordEmbedField `json:"fields,omitempty"`
}

type statusPayload struct {
	Embed   []*discordEmbed `json:"embeds"`
	BotName string          `json:"username"`
}

type lobbyPeers struct {
	Count          int
	Name           string
	RegionName     string
	Comment        string
	RenpoPeers     string
	ZeonPeers      string
	RenpoRoomPeers string
	ZeonRoomPeers  string
	NoForcePeers   string
}

type battlePeers struct {
	RegionName string
	RenpoPeers string
	ZeonPeers  string
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func sortedKeys(m map[uint16]*lobbyPeers) []uint16 {
	keys := make([]uint16, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

//insert Braille Pattern Blank to create newline at ending
func addBlankIfRequired(s string) string {
	if len(s) > 0 {
		return s + "⠀\n"
	}
	return s
}

//Embed limits: Field's value is limited toto 1024 characters
func splitEmbedFieldValues(value string) []string {
	var values []string

	//Split field values into 1024 chunks
	numberOfSplit := int(math.Ceil(float64(len(value)) / 1024))

	splitIndex := 0
	for i := 0; i < numberOfSplit; i++ {
		splitIndex = len(value) / (numberOfSplit - i)
		firstHalf := value[:splitIndex]
		splitIndex = strings.LastIndex(firstHalf, "\n")

		values = append(values, value[:splitIndex])
		// splitIndex += 2
		value = value[splitIndex:]
	}
	//do not create empty fields
	if value != "\n" {
		values = append(values, value)
	}

	return values
}

//Embed limits: There can be up to 25 fields
func splitEmbedFields(fields []*discordEmbedField) [][]*discordEmbedField {
	var divided [][]*discordEmbedField
	chunkSize := 25

	for i := 0; i < len(fields); i += chunkSize {
		end := i + chunkSize

		if end > len(fields) {
			end = len(fields)
		}

		divided = append(divided, fields[i:end])
	}
	return divided
}

func reducePeerStringSize(s string) string {
	//Reduce size by removing userid
	re := regexp.MustCompile("`.*?`\\s")
	s = re.ReplaceAllString(s, "")

	//Reduce size by replacing custom emoji (from 28 char to 1 char)
	s = strings.ReplaceAll(s, "<:gundam:772467554160738355>", "🌎")
	s = strings.ReplaceAll(s, "<:zaku:772467605008023563>", "🪐")

	return s
}

//PublishStatusToDiscord : Update server status to the predefined Discord message thru Web Hook
func (lbs *Lbs) PublishStatusToDiscord() {
	if len(conf.DiscordWebhookURL) == 0 {
		return
	}

	if atomic.CompareAndSwapUint32(&jobIsRunning, 0, 1) {
		go func() {
			publish(lbs)
			atomic.StoreUint32(&jobIsRunning, 0)
		}()
	} else {
		logger.Info("Request blocked!")
		if retryTimer != nil {
			retryTimer.Stop()
		}
		//Retry last blocked request after 0.5s
		retryTimer = time.AfterFunc(time.Second/2, func() {
			logger.Info("Retrying!")
			publish(lbs)
		})
	}
}

func publish(lbs *Lbs) {

	//Stop any retry request since new request appeared
	if retryTimer != nil {
		retryTimer.Stop()
		retryTimer = nil
	}

	//
	// Create battle peer list, for the third embed
	//
	battle := make(map[string]*battlePeers)

	battlePeerCount := 0
	var battlePeersIDs string
	var accumulatedBattlePeersString string

	for _, u := range sharedData.GetMcsUsers() {
		_, exists := battle[u.BattleCode]
		if exists == false {
			battle[u.BattleCode] = new(battlePeers)

			locName, ok := gcpLocationName[u.McsRegion]
			if !ok {
				locName = "Default Server"
			}
			if u.McsRegion == "best" {
				locName = "Best Server"
			}
			battle[u.BattleCode].RegionName = locName
			accumulatedBattlePeersString += locName
		}

		var peer string
		switch u.Side {
		case TeamRenpo:
			peer = fmt.Sprintf("<:gundam:772467554160738355> `%s` %s\n", u.UserID, u.Name)
			battle[u.BattleCode].RenpoPeers += peer
		case TeamZeon:
			peer = fmt.Sprintf("<:zaku:772467605008023563> `%s` %s\n", u.UserID, u.Name)
			battle[u.BattleCode].ZeonPeers += peer
		}
		accumulatedBattlePeersString += peer

		battlePeerCount++
		battlePeersIDs += u.UserID
	}

	//
	// Create lobby peer list, for the second embed
	//
	var plazaPeers string

	lobby := make(map[uint16]*lobbyPeers)

	plazaPeerCount := 0
	lobbyPeerCount := 0

	var accumulatedLobbyPeersString string

	lbs.Locked(func(lbs *Lbs) {
		for _, u := range lbs.userPeers {

			//Already in battle, hidden from lobby
			if strings.Contains(battlePeersIDs, u.UserID) {
				continue
			}

			if u.Lobby == nil {
				plazaPeers += fmt.Sprintf("`%s` %s\n", u.UserID, u.Name)
				accumulatedLobbyPeersString += plazaPeers
				plazaPeerCount++
			} else {
				_, exists := lobby[u.Lobby.ID]
				if exists == false {
					lobby[u.Lobby.ID] = new(lobbyPeers)
					lobby[u.Lobby.ID].Name = u.Lobby.Name

					locName, ok := gcpLocationName[u.Lobby.McsRegion]
					if !ok {
						locName = "Default Server"
					}
					if u.Lobby.McsRegion == "best" {
						locName = "Best Server"
					}
					lobby[u.Lobby.ID].RegionName = locName

					var comment string
					if strings.Contains(u.Lobby.Comment, "TeamShuffle") {
						comment += "🔀"
					}
					if strings.Contains(u.Lobby.Comment, "For JP vs HK") {
						comment += "( 🇯🇵 vs 🇭🇰 )"
					}
					if strings.Contains(u.Lobby.Comment, "Private Room") {
						comment += "🔒"
					}
					if strings.Contains(u.Lobby.Comment, "No 375 Cost MS") {
						comment += "⛔375"
					}
					if strings.Contains(u.Lobby.Comment, "3R") {
						comment += "3️⃣"
					}

					lobby[u.Lobby.ID].Comment = comment
				}

				var readyColor string
				if contains(u.Lobby.EntryUsers, u.UserID) {
					readyColor = "🟢"
				} else {
					readyColor = "🔴"
				}
				if u.Room != nil {
					readyColor = "📢"
				}
				var peer string
				switch u.Team {
				case TeamRenpo:
					peer = fmt.Sprintf("<:gundam:772467554160738355>%s `%s` %s\n", readyColor, u.UserID, u.Name)
					if u.Room == nil {
						lobby[u.Lobby.ID].RenpoPeers += peer
					} else {
						lobby[u.Lobby.ID].RenpoRoomPeers += peer
					}
				case TeamZeon:
					peer = fmt.Sprintf("<:zaku:772467605008023563>%s `%s` %s\n", readyColor, u.UserID, u.Name)
					if u.Room == nil {
						lobby[u.Lobby.ID].ZeonPeers += peer
					} else {
						lobby[u.Lobby.ID].ZeonRoomPeers += peer
					}
				default:
					peer = fmt.Sprintf("❔⚫ `%s` %s\n", u.UserID, u.Name)
					lobby[u.Lobby.ID].NoForcePeers += peer
				}
				accumulatedLobbyPeersString += peer

				lobby[u.Lobby.ID].Count++
			}
			lobbyPeerCount++
		}
	})

	if lobby[2] != nil {
		for i := 0; i < 4; i++ {
			lobby[2].NoForcePeers += lobby[2].NoForcePeers
			lobby[2].RenpoPeers += lobby[2].RenpoPeers
		}
	}

	//
	// Handle oversized string
	//
	embedAccumulatedLobbyStringLength := len(accumulatedLobbyPeersString)
	logger.Info("Lobby string Size", zap.Int("length", embedAccumulatedLobbyStringLength))
	if embedAccumulatedLobbyStringLength > 6000 {
		plazaPeers = reducePeerStringSize(plazaPeers)

		for _, l := range lobby {
			l.RenpoPeers = reducePeerStringSize(l.RenpoPeers)
			l.ZeonPeers = reducePeerStringSize(l.ZeonPeers)
			l.RenpoRoomPeers = reducePeerStringSize(l.RenpoRoomPeers)
			l.ZeonRoomPeers = reducePeerStringSize(l.ZeonRoomPeers)
			l.NoForcePeers = reducePeerStringSize(l.NoForcePeers)
		}
		logger.Info("Lobby string size reduced!")
	}
	embedAccumulatedBattleStringLength := len(accumulatedBattlePeersString)
	logger.Info("Battle string Size", zap.Int("length", embedAccumulatedBattleStringLength))
	if embedAccumulatedBattleStringLength > 6000 {
		for _, b := range battle {
			b.RenpoPeers = reducePeerStringSize(b.RenpoPeers)
			b.ZeonPeers = reducePeerStringSize(b.ZeonPeers)
		}
		logger.Info("Battle string size reduced!")
	}

	//
	// Start to create JSON payload
	//
	payload := new(statusPayload)
	payload.BotName = "Live Status"

	//
	//1st Embed, online count
	//
	payload.Embed = append(payload.Embed, &discordEmbed{
		Title:     fmt.Sprintf("**同時接続数 %d 人 **", lobbyPeerCount+battlePeerCount),
		Color:     52224,
		Footer:    &discordEmbedFooter{Text: "🕒"},
		Timestamp: fmt.Sprintf(time.Now().UTC().Format("2006-01-02T15:04:05.000Z")),
	})

	//
	//2nd Embed, lobby count
	//

	//1st Field is always Plaza
	var lobbyFields []*discordEmbedField
	if plazaPeerCount > 0 {
		for i, v := range splitEmbedFieldValues(plazaPeers) {
			name := "⠀"
			if i == 0 {
				name = fmt.Sprintf("**Plaza － %d 人**", plazaPeerCount)
			}

			lobbyFields = append(lobbyFields, &discordEmbedField{
				Name:  name,
				Value: v,
			})
		}
	}

	//Following Fields would be all lobbies
	//Use sortedKeys to fix the ordering
	for _, i := range sortedKeys(lobby) {
		l := lobby[i]

		value := addBlankIfRequired(l.RenpoPeers+l.ZeonPeers) + addBlankIfRequired(l.RenpoRoomPeers+l.ZeonRoomPeers) + addBlankIfRequired(l.NoForcePeers)

		for i, v := range splitEmbedFieldValues(value) {
			name := "⠀"
			if i == 0 {
				name = fmt.Sprintf("**%s － %d 人\n%s %s**", l.Name, l.Count, l.RegionName, l.Comment)
			}

			lobbyFields = append(lobbyFields, &discordEmbedField{
				Name:  name,
				Value: v,
			})
		}

	}
	for i, fields := range splitEmbedFields(lobbyFields) {
		description := ""
		if i == 0 {
			description = fmt.Sprintf("🌐 **待機中 %d 人**", lobbyPeerCount)
		}
		payload.Embed = append(payload.Embed, &discordEmbed{
			Description: description,
			Color:       24041,
			Fields:      fields,
		})
	}

	//
	//3rd Embed, battle count
	//
	var battleFields []*discordEmbedField
	for _, b := range battle {

		value := addBlankIfRequired(b.RenpoPeers + b.ZeonPeers)

		for i, v := range splitEmbedFieldValues(value) {
			name := "⠀"
			if i == 0 {
				name = b.RegionName
			}

			battleFields = append(battleFields, &discordEmbedField{
				Name:  name,
				Value: v,
			})
		}
	}
	for i, fields := range splitEmbedFields(battleFields) {
		description := ""
		if i == 0 {
			description = fmt.Sprintf("💥 **戦闘中 %d 人**", battlePeerCount)
		}
		payload.Embed = append(payload.Embed, &discordEmbed{
			Description: description,
			Color:       13179394,
			Fields:      fields,
		})
	}

	//
	// Create the json
	//
	var jsonData []byte
	jsonData, err := json.Marshal(payload)

	if err != nil {
		logger.Error("Failed to create Discord JSON", zap.Error(err))
		return
	}
	jsonString := string(jsonData)
	logger.Info(jsonString, zap.Int("length", len(jsonString)))

	send(jsonData)

}

//
// Send to Discord
//
func send(jsonData []byte) {

	req, err := http.NewRequest("PATCH", conf.DiscordWebhookURL, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("Failed to publish to Discord", zap.Error(err))
		return
	}
	defer resp.Body.Close()

	logger.Info("Discord Webhook sent", zap.String("Status", resp.Status))
	if resp.Status == "400 Bad Request" {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logger.Error("Failed to read response body", zap.Error(err))
			return
		}
		logger.Error("Failed to create Discord JSON", zap.String("Error:", string(body)))
	} else if resp.Status == "429 Too Many Requests" {

		resetepochTime := resp.Header.Get("x-ratelimit-reset")
		sec, _ := strconv.ParseInt(resetepochTime, 10, 64)
		logger.Info("Rate limit", zap.Int64("x-ratelimit-reset", sec))

		retryTimer = time.AfterFunc(time.Unix(sec, 0).Sub(time.Now()), func() {
			logger.Info("Retrying last request", zap.Int64("epoch", time.Now().Unix()))
			send(jsonData)
		})
	}

}
