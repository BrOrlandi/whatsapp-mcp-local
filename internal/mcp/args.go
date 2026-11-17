package mcp

// A value a caller gives never becomes a wacli flag. wacli honours flags
// anywhere on the line, after positional arguments too (checked with the
// pinned 0.26.1: `contacts check 5511… --help` prints the help), so:
//
//   - a value given after its own flag (--to <value>, --chat <value>,
//     --jid <value>, --user <value>) is read as that flag's value whatever it
//     looks like (checked with 0.26.1: `messages list --chat --store=/x` keeps
//     the store and `--chat -h` prints no help). These values are passed as
//     they were in 1.8.0: a contact or group name, a JID, a phone number,
//     anything wacli resolves;
//   - positional values (the numbers to check, a search query) go after "--",
//     which wacli takes as the end of its flags.
//
// No caller value is ever placed on the line in any other position.
