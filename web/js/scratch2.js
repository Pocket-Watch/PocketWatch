import { Player } from "./custom_player.js"
import * as api from "./scratch2_api.js";

class Room {
    constructor() {
        let video0 = document.getElementById("video0");
        this.player = new Player(video0);

        this.urlArea = {
            urlInput:     document.getElementById("url_input_box"),
            titleInput:   document.getElementById("url_title_input"),
            refererInput: document.getElementById("url_dropdown_referer_input"),

            dropdownButton:    document.getElementById("url_dropdown_button"),
            resetButton:       document.getElementById("url_reset_button"),
            setButton:         document.getElementById("url_set_button"),
            addPlaylistButton: document.getElementById("url_add_playlist_button"),

            dropdownContainer: document.getElementById("url_dropdown_container"),
        };

        this.dropdownIsDown = false;
        this.urlArea.dropdownContainer.style.display = "none"

        this.connectionId = 0;
        this.user = null;
        this.token = null;
    }

    attachPlayerEvents() {
        this.player.onControlsPlay(() => {
            api.playerPlay(this.player.getCurrentTime());
        })

        this.player.onControlsPause(() => {
            api.playerPause(this.player.getCurrentTime());
        })

        this.player.onControlsSeeked((timestamp) => {
            api.playerSeek(timestamp);
        })

        this.player.onControlsSeeking((timestamp) => {
            console.log("User seeking to", timestamp);
        })

        this.player.onPlaybackError((event) => {
            this.player.setToast(event.name + " - " + event.message);
        })
    }

    resetUrlAreaElements() {
        this.urlArea.urlInput.value = "";
        this.urlArea.titleInput.value = "";
        this.urlArea.refererInput.value = "";
    }

    attachHtmlEvents() {
        this.urlArea.dropdownButton.onclick = () => {
            let button = this.urlArea.dropdownButton;
            let div = this.urlArea.dropdownContainer;

            if (this.dropdownIsDown) {
                button.textContent = "▼";
                div.style.display = "none";
                this.dropdownIsDown = false;
            } else {
                button.textContent = "▲";
                div.style.display = "";
                this.dropdownIsDown = true;
            }
        }

        this.urlArea.resetButton.onclick = () => {
            this.resetUrlAreaElements();
        }

        this.urlArea.setButton.onclick = () => {
            const entry = {
                id: 0,
                url: this.urlArea.urlInput.value,
                title: this.urlArea.titleInput.value,
                user_id: 0,
                use_proxy: false,
                referer_url: this.urlArea.refererInput.value,
                created: new Date,
            };

            api.playerSet(entry);
            this.resetUrlAreaElements();
        }
    }

    async loadOrCreateUser() {
        let token = localStorage.getItem("token");
        if (!token) {
            token = await api.userCreate();
            localStorage.setItem("token", token)
            this.user = await api.userVerify(token);
        } else {
            let user = await api.userVerify(token);
            if (!user) {
                token = await api.userCreate();
                localStorage.setItem("token", token)
                user = await api.userVerify(token);
            }

            this.user = user;
        }

        this.token = token;
        api.setToken(token);
    }

    async loadPlayerData() {
        let state = await api.playerGet();
        this.player.setAutoplay(state.player.autoplay);
        this.player.setLoop(state.player.looping);
    }

    resyncPlayer(timestamp, userId) {
        const MAX_DESYNC = 1.5;
        let desync = timestamp - this.player.getCurrentTime();

        if (userId == 0) {
            console.info("INFO: Received resync event from SERVER at", timestamp, "with desync:", desync);
        } else {
            console.info("INFO: Received resync event from USER id", userId, "at", timestamp, "with desync:", desync);
        }

        if (Math.abs(desync) > MAX_DESYNC) {
            let diff = Math.abs(desync) - MAX_DESYNC
            console.warn("You are desynced! MAX_DESYNC(" + MAX_DESYNC + ") exceeded by:", diff, "Trying to resync now!");
            this.player.seek(timestamp);
        }
    }

    async subscribeToServerEvents() {
        let events = new EventSource("/watch/api/events?token=" + this.token);

        events.addEventListener("welcome", (event) => {
            let connectionId = JSON.parse(event.data);
            console.info("INFO: Received a welcome request with connection id: ", connectionId);
            this.connectionId = connectionId;

            api.setConnectionId(this.connectionId);
        });

        events.addEventListener("playerset", (event) => {
            let response = JSON.parse(event.data);
            console.info("INFO: Received player set event: ", response);

            let entry = response.new_entry;
            this.player.setVideoTrack(entry.url);
            this.player.setTitle(entry.title);
        });

        events.addEventListener("sync", (event) => {
            let data = JSON.parse(event.data);
            if (!data) {
                console.error("ERROR: Failed to parse event data")
                return;
            }

            let timestamp = data.timestamp;
            let userId = data.user_id;

            switch (data.action) {
                case "play": {
                    this.player.setToast("User clicked play.");
                    this.resyncPlayer(timestamp, userId);
                    this.player.play();
                } break;

                case "pause": {
                    this.player.setToast("User clicked pause.");
                    this.resyncPlayer(timestamp, userId);
                    this.player.pause();
                } break;

                case "seek": {
                    this.player.setToast("User seeked.");
                    this.player.seek(timestamp);
                } break;

                default: {
                    console.error("ERROR: Unknown sync action found", data.action)
                } break;
            }
        });

    }

    async connectToServer() {
        await this.loadOrCreateUser();
        await this.loadPlayerData();
        // await this.loadPlaylistData();
        await this.subscribeToServerEvents();
    }
}

async function main() {
    let room = new Room()
    room.attachPlayerEvents();
    room.attachHtmlEvents();
    await room.connectToServer();
}

main();
