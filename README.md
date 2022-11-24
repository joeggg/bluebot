# Bluebot

The newer Go version of my original Discord bot in Node.js. Just has a small collection of different commands that have been useful on my Discord server.

## Commands

All commands start with the `%` symbol followed by a keyword: `%<command>`.

### **%yt**

Joins the voice channel you are in and plays audio from YoutTube links or search terms, with different commands starting with `%yt` followed by a second keyword:

Usage:
- `%yt play <URL or search term>` Add a video or playlist to the queue and start playing
- `%yt queue <URL or search term>` Add to the queue once already playing
- `%yt next` Skip forward to the next track
- `%yt pause` Pause the music
- `%yt resume` Resume the music
- `%yt stop` Stop playing and cancel the whole queue
- `%yt list` Show the current queue

### **%civ**

Gives a selection of random Civilizations 5 civs to play for a given set of players. Can restrict to give only certain tiers of civ. Intended as a nicer way of more randomly choosing what to play without having to random in-game. Number of civs given is set in config (default is 3)

Usage:
- `%civ <player1> <player2> ...` Generate a selection of civs for the given player names
- `%civ` Regenerate the set of civs based on the last players given in this text channel. Settings persist for 5 minutes - can be set in config
- `%civ tiers <min/max>-<min/max>` Set the min/max tiers to those given (order doesn't matter). The full range of tiers is 1-8. 

### **%say**

Sends a random phrase as a message from a list provided in the folder `data/phrases/say.json`. See the `Phrases` section below for details of custom phrases.

Usage:
- `%say`

### **%tell**
Joins the voice channel you are in and will say a message you provide with the Google Text-to-Speech API (https://cloud.google.com/text-to-speech). Uses a default preset but others can be added and selected. See the `Voice presets` section for details on adding your own presets.

Usage:
- `%tell <message>`
- `setvoice <preset>`


## Installation
Can be installed as a Linux systemd service to the host system or a remote target. Also can be installed locally within the repo folder for testing.
The system you install to must have Go installed as well as `libopus-dev`.

### Systemd
Either run `sudo scripts/install.sh` for a local install or `scripts/deploy.sh` and pass the ssh target in as the main argument e.g. `./scripts/deploy.sh joe@myserver`.
Ensure you have sudo access over ssh, otherwise manually move the files across and run the install script. The install script creates a user for Bluebot, builds and copies the executable and data/config files their correct install locations, makes a log file at `/var/log/bluebot/logfile.log`, and installs as a systemd service.

You will need to add your own discord token to the file `/etc/bluebot/token.txt` and a Google YouTube API key file `/etc/bluebot/google_token.json`. If done after deploying you must restart the service.

### Local test
Run `scripts/install.sh test` in a shell in the repo folder. The necessary folders will be created and the executable built locally within the repo. Then just run `./run.sh` to start the bot. 

As with the systemd install, you must have the 2 required tokens at `./token/token.txt` and `./token/google_token.json`. 


## Image Commands

Bluebot allows for loading custom commands for generating memes. Each command will take an image and paste text you provide onto the image. In a file `data/images.json` you specify the command name, filename and (x, y) pixel coordinates of location of the text (the text is centred on the pixel). The files should be placed in `data/images/*.png`

JSON file format:
```
{
    "disgust": {
        "filename": "disgust.png",
        "text_x": 180,
        "text_y": 180
    }
    ...
}
```
Example output in Discord:
```
> %disgust you
```
![image](https://user-images.githubusercontent.com/47352958/203411509-a7ea1653-733b-4114-9800-f15c68dd4497.png)


## Phrases

In certain instances, the bot will message the text channel with a random phrase from lists of phrases you have to provide. These lists should go in `data/phrases/<name>.json` and will be copied to the correct location when installed (or left there if test install).

The current lists in use are:
- `say.json` for responses to the `%say` command
- `wrongcommand.json` for responses to writing an incorrect command keyword

If no phrases are provided for one of the given situations, the bot will just response with `Hello!`.

JSON file format:
```
{
    "data": [
        "phrase 1...",
        "phrase 2...",
        ...
    ]
}
```


## Voice presets

Place a JSON file at `data/voice_presets.json` before installing to load your own presets. At least a default voice is required for the `%tell` command to work. The keys for the preset correspond to the settings seen on the Google page for Text-to-Speech linked above.

JSON file format:
```
{
    "default": {
        "language": "en-AU",
        "name": "en-AU-Wavenet-A",
        "pitch": 1,
        "rate": 1,
        "gender": "MALE"
    },
    <voice name>: {
        ...
}
```
