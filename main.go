package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// DiscordAPIToken is the flag for the Discord API token
	DiscordAPIToken = "discord-api-token"
)

var botCmd = &cobra.Command{
	Use:   "aybot",
	Short: "aybot is Aybaze's little helper bot.",
	Long:  "aybot is Aybaze's little helper bot.",
	Run:   doCmd,
}

func init() {
	cobra.OnInitialize(initConfig)

	botCmd.Flags().StringP(DiscordAPIToken, "t", "", "The token for Discord integration")
	viper.BindPFlag(DiscordAPIToken, botCmd.Flags().Lookup(DiscordAPIToken))
}

func initConfig() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

var token string
var buffer = make([][]byte, 0)

var homeChannel string

var presences = make(map[string]string)

func doCmd(cmd *cobra.Command, args []string) {
	token := viper.GetString(DiscordAPIToken)

	if token == "" {
		log.Println("No Discord API token provided.")
		return
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Println("Error creating Discord session: ", err)
		return
	}

	//session.LogLevel = discordgo.LogDebug

	session.AddHandler(voiceStateUpdate)
	session.AddHandler(presenceUpdate)

	session.AddHandler(ready)

	session.AddHandler(guildCreate)

	// Open the websocket and begin listening.
	err = session.Open()
	if err != nil {
		log.Println("Error opening Discord session: ", err)
	}

	// Wait here until CTRL-C or other term signal is received.
	log.Println("Aybot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session.
	session.Close()
}

func main() {
	log.SetOutput(os.Stdout)

	if err := botCmd.Execute(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

// This function will be called when the bot receives the "ready" event from Discord.
func ready(s *discordgo.Session, event *discordgo.Ready) {
	// Set the playing status.
	s.UpdateStatus(0, "ready to serve")
}

// This function will be called when the voice state changes,
func voiceStateUpdate(s *discordgo.Session, m *discordgo.VoiceStateUpdate) {
	// dont update if its bot himself
	if m.UserID == s.State.User.ID {
		return
	}

	user, _ := s.GuildMember(m.GuildID, m.UserID)

	if m.ChannelID == "" {
		// user left
		if homeChannel != "" {
			data := &discordgo.MessageEmbed{
				Description: fmt.Sprintf("... left voice channel."),
				Color:       0x96281b,
				//Timestamp: time.Now().Format(time.RFC3339),
				Author: &discordgo.MessageEmbedAuthor{Name: user.User.Username, IconURL: user.User.AvatarURL("128")},
			}

			_, _ = s.ChannelMessageSendEmbed(homeChannel, data)
		}
	} else {
		// user joined
		if homeChannel != "" {
			data := &discordgo.MessageEmbed{
				Description: fmt.Sprintf("... joined voice channel."),
				Color:       0x1e824c,
				//Timestamp: time.Now().Format(time.RFC3339),
				Author: &discordgo.MessageEmbedAuthor{Name: user.User.Username, IconURL: user.User.AvatarURL("128")},
			}

			_, _ = s.ChannelMessageSendEmbed(homeChannel, data)
		}
	}
}

func presencesReplace(s *discordgo.Session, m *discordgo.PresencesReplace) {
	log.Printf("Presence replace: %+v\n", m)
}

func presenceUpdate(s *discordgo.Session, m *discordgo.PresenceUpdate) {
	log.Printf("Presence update: %+v\n", m)

	member, _ := s.GuildMember(m.GuildID, m.User.ID)

	oldPresence := presences[m.User.ID]
	newPresence := ""

	if m.Game != nil {
		newPresence = m.Game.Name
	}

	changed := oldPresence != newPresence

	// nothing really has changed, ignore
	// this is mostly if a members goes to/from afk to online, etc.
	if !changed {
		return
	}

	if newPresence == "" {
		// user stopped playing
		if homeChannel != "" {
			data := &discordgo.MessageEmbed{
				Description: fmt.Sprintf("... stopped playing **%s**.", oldPresence),
				Color:       0x96281b,
				//Timestamp: time.Now().Format(time.RFC3339),
				Author: &discordgo.MessageEmbedAuthor{Name: member.User.Username, IconURL: member.User.AvatarURL("128")},
			}

			_, _ = s.ChannelMessageSendEmbed(homeChannel, data)
		}
	} else {
		// user started playing
		if homeChannel != "" {
			data := &discordgo.MessageEmbed{
				Description: fmt.Sprintf("... started playing **%s**.", newPresence),
				Color:       0x1e824c,
				//Timestamp: time.Now().Format(time.RFC3339),
				Author: &discordgo.MessageEmbedAuthor{Name: member.User.Username, IconURL: member.User.AvatarURL("128")},
			}

			_, _ = s.ChannelMessageSendEmbed(homeChannel, data)
		}
	}

	// update presence
	presences[m.User.ID] = newPresence
}

// This function will be called (due to AddHandler above) every time a new
// guild is joined.
func guildCreate(s *discordgo.Session, event *discordgo.GuildCreate) {
	if event.Guild.Unavailable {
		return
	}

	// update presences
	for _, presence := range event.Guild.Presences {
		if presence.Game != nil {
			presences[presence.User.ID] = presence.Game.Name
		}
	}

	for _, channel := range event.Guild.Channels {
		if channel.Name == "general" {
			_, _ = s.ChannelMessageSend(channel.ID, "Aybot is ready to serve.")
			homeChannel = channel.ID
			return
		}
	}
}
