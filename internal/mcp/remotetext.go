package mcp

// An agent on another computer, through the server mode's public address,
// reads the tools as they are for it: the server's files are not on its
// computer, a path on the server means nothing to it, and the links it gets
// are on the public address. These descriptions replace the local ones in
// its tools/list; programs on the server itself keep the local ones.
var remoteDescriptions = map[string]string{
	"send_media_message": "Send an image, video, audio, document or sticker, from an https URL on the internet that the server downloads (a path on your computer cannot be read by the server). A sticker must be a WebP image, still or animated, and carries no caption; stickers already received or sent can be reused: download_media gives them as WebP.",
	"download_media":     "Get the media of an indexed message, exactly as WhatsApp delivered it: original size and format, nothing converted. By default the content comes inside this result (a picture as an image, an audio as audio, anything else as a file), which puts the whole file into the conversation. When it is too large for you or your client cannot take it, call again with link true: that returns a URL on the server's public address, valid for ten minutes and needing no credential, with a curl command that saves it. Files over 20 MiB always come as a link.",
	"export_messages":    "Write messages to a file on the server, one JSON message per line (NDJSON), oldest first, and return a URL on the server's public address to download it, valid for ten minutes, with the count and the period. For analysing a whole archive with a script without loading it into the conversation. Voice notes carry their transcript when one is kept.",
}

// remoteParams replaces parameter descriptions the same way.
var remoteParams = map[string]map[string]string{
	"send_media_message": {"url": "https URL of the file on the internet; paths and addresses inside the server's network are refused."},
	"download_media":     {"link": "Return a temporary download URL on the server's public address instead of the content."},
}

// forRemote rewrites a tool definition for an agent on another computer.
func forRemote(def map[string]any) {
	name, _ := def["name"].(string)
	if d, ok := remoteDescriptions[name]; ok {
		def["description"] = d
	}
	params, ok := remoteParams[name]
	if !ok {
		return
	}
	schema, _ := def["inputSchema"].(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	for p, d := range params {
		if prop, ok := props[p].(map[string]any); ok {
			prop["description"] = d
		}
	}
}
