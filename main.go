package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/schettle/rotacop/rota"

	"github.com/nlopes/slack"
)

func main() {

	token := "token"
	api := slack.New(token)
	api.SetDebug(true)

	rtm := api.NewRTM()
	rotaHelper := rota.NewRotaHelper(rtm)

	go rtm.ManageConnection()

	rtm.SendMessage(rtm.NewOutgoingMessage("Come quietly or there will be... trouble.", rotaHelper.ChannelNameToID("schettletest")))
	rotaHelper.Monitor(rotaHelper.GetRotas())

	for {
		select {
		case msg := <-rtm.IncomingEvents:
			switch ev := msg.Data.(type) {

			case *slack.MessageEvent:
				// fmt.Printf("Message: %v\n", ev)
				info := rtm.GetInfo()
				prefix := fmt.Sprintf("<@%s> ", info.User.ID)

				if ev.User != info.User.ID && strings.HasPrefix(ev.Text, prefix) {
					respond(rtm, ev, prefix, rotaHelper)
				}

			default:
				//Take no action
			}
		}
	}
}

func respond(rtm *slack.RTM, msg *slack.MessageEvent, prefix string, rotaHelper rota.RotaHelper) {
	text := msg.Text
	text = strings.TrimPrefix(text, prefix)
	text = strings.TrimSpace(text)
	text = strings.ToLower(text)

	var validRC = regexp.MustCompile(`(?i)\sRC(\s|\?)`)
	var validBughat = regexp.MustCompile(`(?i)\s(Bughat|bug\shat|bh)(\s|\?)`)

	if validRC.MatchString(text) {
		rtm.SendMessage(rtm.NewOutgoingMessage(rotaHelper.GetUserOnDuty("RC", false), msg.Channel))
	} else if validBughat.MatchString(text) {
		rtm.SendMessage(rtm.NewOutgoingMessage(rotaHelper.GetUserOnDuty("BH", false), msg.Channel))
	} else {
		rtm.SendMessage(rtm.NewOutgoingMessage("I didn't quite get that, please try again.", msg.Channel))
	}
}
