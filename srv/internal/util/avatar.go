package util

var AvatarEmojis = map[string]string{
	"🐱":    "cat",
	"🐶":    "dog",
	"🐉":    "dragon",
	"👽":    "alien",
	"🤖":    "robot",
	"👻":    "ghost",
	"🧙‍♂️": "wizard",
	"👤":    "",
}

func AvatarEmojiToText(emoji string) string {
	if text, ok := AvatarEmojis[emoji]; ok {
		return text
	}
	return ""
}

func AvatarTextToEmoji(text string) string {
	for emoji, t := range AvatarEmojis {
		if t == text {
			return emoji
		}
	}
	return ""
}
