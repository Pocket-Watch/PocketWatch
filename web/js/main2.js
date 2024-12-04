import { Player } from "./custom_player.js"
import * as api from "./api2.js";

const SERVER_ID = 0;

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
            proxyToggle:       document.getElementById("proxy_toggle"),
        };

        this.usersArea = {
            userList: document.getElementById("users_list"),

            onlineCount:  document.getElementById("users_online_count"),
            offlineCount: document.getElementById("users_offline_count"),
        };

        this.proxyEnabled = false;
        this.dropdownIsDown = false;

        /// Current connection id.
        this.connectionId = 0;

        /// Server User structure.
        this.currentUser = null;

        /// User token string.
        this.token = "";

        /// List of all users
        this.allUsers = [];
    }

    sliderTesting() {
        let calculateProgress = (event, element) => {
            let rect = element.getBoundingClientRect();
            let offsetX;

            if (event.touches) {
                offsetX = event.touches[0].clientX - rect.left;
            } else {
                offsetX = event.clientX - rect.left;
            }

            // Ensure the touch doesn't exceed slider bounds
            if (offsetX < 0) offsetX = 0;
            if (offsetX > rect.width) offsetX = rect.width;

            let progress = offsetX / rect.width;
            if (isNaN(progress)) {
                progress = 0;
            }

            return progress;
        }

        let sliderMove = (e) => {
            let bar = document.getElementById("cp_bar");
            let progress = calculateProgress(e, bar);
            console.log(progress);

            document.getElementById("cp_progress").style.width = progress * 100 + "%"; 

            let width = bar.clientWidth;

            let thumb = document.getElementById("cp_thumb");
            let thumb_width = thumb.clientWidth / 2.0;
            let thumb_left = width * progress - thumb_width;

            thumb.style.marginLeft = thumb_left + "px";
        }

        let removeSliderEvents = () => {
            document.removeEventListener("mousemove", sliderMove);
            document.removeEventListener("mouseup", removeSliderEvents);
        }

        document.getElementById("cp_slider").onmousedown = (e) => {
            document.addEventListener("mousemove", sliderMove);
            document.addEventListener("mouseup", removeSliderEvents);
            sliderMove(e);
        };

        // document.getElementById("cp_input").oninput = (e) => {
        //     let t = e.target;
        //     // console.log(e.clientX);
        //
        //     let w = document.getElementById("cp_bar").clientWidth;
        //     let p = Number(t.value);
        //
        //     let s = document.getElementById("cp_thumb").clientWidth / 2.0;
        //     let ml = ((p * (w - s)) / w) * 100;
        //
        //     console.log(p);
        //
        //     document.getElementById("cp_progress").style.width = p * w + "px"; 
        //     document.getElementById("cp_thumb").style.marginLeft = ml + "%";
        //
        //     console.log(t.value);
        // }
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
            this.player.setToast("ERROR: Something went wrong, press F12 to see what happened");
            console.error(event.name + ":", event.message);
        })
    }

    resetUrlAreaElements() {
        this.urlArea.urlInput.value = "";
        this.urlArea.titleInput.value = "";
        this.urlArea.refererInput.value = "";

        this.proxyEnabled = false;
        this.urlArea.proxyToggle.classList.remove("proxy_active");
    }

    sendNewEntry() {
        const entry = {
            id:          0,
            url:         this.urlArea.urlInput.value,
            title:       this.urlArea.titleInput.value,
            user_id:     0,
            use_proxy:   this.proxyEnabled,
            referer_url: this.urlArea.refererInput.value,
            created:     new Date,
        };

        api.playerSet(entry);
    }

    attachHtmlEvents() {
        this.urlArea.dropdownButton.onclick = () => {
            let button = this.urlArea.dropdownButton;
            let div = this.urlArea.dropdownContainer;

            if (this.dropdownIsDown) {
                button.textContent = "▼";
            } else {
                button.textContent = "▲";
            }

            div.classList.toggle("url_dropdown_collapsed");
            div.classList.toggle("url_dropdown_expanded");

            this.dropdownIsDown = !this.dropdownIsDown;
        }

        this.urlArea.resetButton.onclick = () => {
            this.resetUrlAreaElements();
        }

        this.urlArea.setButton.onclick = () => {
            this.sendNewEntry();
            this.resetUrlAreaElements();
        }

        this.urlArea.urlInput.onkeypress = (event) => {
            if (event.key == "Enter") {
                this.sendNewEntry();
                this.resetUrlAreaElements();
            }
        }

        this.urlArea.proxyToggle.onclick = () => {
            this.urlArea.proxyToggle.classList.toggle("proxy_active");
            this.proxyEnabled = !this.proxyEnabled;
        }
    }

    async createNewAccount() {
        this.token = await api.userCreate();
        api.setToken(this.token);
        localStorage.setItem("token", this.token);

        this.currentUser = await api.userVerify();
    }

    async authenticateAccount() {
        this.token = localStorage.getItem("token");
        api.setToken(this.token);

        this.currentUser = await api.userVerify();
        if (!this.currentUser) {
            await this.createNewAccount();
        }
    }

    async loadPlayerData() {
        let state = await api.playerGet();
        this.player.setAutoplay(state.player.autoplay);
        this.player.setLoop(state.player.looping);

        let entry = state.entry;
        this.player.setVideoTrack(entry.url);
        this.player.setTitle(entry.title);
    }

    async loadUsersData() {
        this.allUsers = await api.userGetAll();

        for (var i = 0; i < this.allUsers.length; i++) {
            if (this.allUsers[i].id == this.currentUser.id) {
                this.allUsers[i].connections += 1;
                break;
            }
        }
        this.updateUsersArea();
    }

    createUserBox(user) {
        let userBox = document.createElement("div");
        userBox.className = "user_box";

        // user_box_top
        let userBoxTop = document.createElement("div");
        userBoxTop.className = "user_box_top";

        { // user_box_top img
            let userAvatar = document.createElement("img");
            userAvatar.src = user.avatar ? user.avatar : "img/default_avatar.png"; 
            userBoxTop.append(userAvatar);

            let changeAvatarButton = document.createElement("button");
            changeAvatarButton.className = "user_box_change_avatar";
            changeAvatarButton.textContent = "E";
            changeAvatarButton.onclick = () => {
                var input = document.createElement('input');
                input.type = "file";

                input.onchange = event => { 
                    var file = event.target.files[0]; 
                    console.log("Picked file:", file);
                    api.userUpdateAvatar(file).then(newAvatar => {
                        if (newAvatar) {
                            userAvatar.src = newAvatar;
                        }
                    });
                }

                input.click();
            };

            userBoxTop.append(changeAvatarButton);
        }

        userBox.append(userBoxTop);

        // user_box_bottom
        let userBoxBottom = document.createElement("div");
        userBoxBottom.className = "user_box_bottom";

        { // user_box_bottom input + user_box_edit_name_button
            let usernameInput = document.createElement("input");
            usernameInput.readOnly = true;
            usernameInput.value = user.username;

            userBoxBottom.append(usernameInput);

            if (user.id == this.currentUser.id) {
                usernameInput.addEventListener("focusout", () => {
                    usernameInput.readOnly = true;
                    api.userUpdateName(usernameInput.value);
                });

                usernameInput.addEventListener("keypress", (event) => {
                    if (event.key === "Enter") {
                        usernameInput.readOnly = true;
                        api.userUpdateName(usernameInput.value);
                    }
                });

                // usernameInput.addEventListener("dblclick", (event) => {
                //     usernameInput.readOnly = false;
                //     usernameInput.focus();
                // });

                let editNameButton = document.createElement("button");
                editNameButton.className = "user_box_edit_name_button";
                editNameButton.onclick = () => {
                    usernameInput.readOnly = false;
                    usernameInput.focus();
                };

                { // user_box_edit_name_button svg
                    let editSvg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
                    editSvg.setAttribute("viewBox", "0 0 16 16");

                    let path1 = document.createElementNS("http://www.w3.org/2000/svg", "path");
                    path1.setAttribute("d", "M8.29289 3.70711L1 11V15H5L12.2929 7.70711L8.29289 3.70711Z");
                    editSvg.append(path1);

                    let path2 = document.createElementNS("http://www.w3.org/2000/svg","path");
                    path2.setAttribute("d", "M9.70711 2.29289L13.7071 6.29289L15.1716 4.82843C15.702 4.29799 16 3.57857 16 2.82843C16 1.26633 14.7337 0 13.1716 0C12.4214 0 11.702 0.297995 11.1716 0.828428L9.70711 2.29289Z");
                    editSvg.append(path2);

                    editNameButton.append(editSvg);
                }

                userBoxBottom.append(editNameButton);
            }
        }

        userBox.append(userBoxBottom);

        return userBox;
    }

    updateUsersArea() {
        let onlineUsers = [];
        let offlineUsers = [];

        for (var i = 0; i < this.allUsers.length; i++) {
            let newUserBox = this.createUserBox(this.allUsers[i]);

            if (this.allUsers[i].connections > 0) {
                newUserBox.classList.add("user_box_online");
                onlineUsers.push(newUserBox);
            } else {
                newUserBox.classList.add("user_box_offline");
                offlineUsers.push(newUserBox);
            }
        }

        this.usersArea.onlineCount.textContent = onlineUsers.length;
        this.usersArea.offlineCount.textContent = offlineUsers.length;

        // NOTE(kihau): 
        //     Those should not be removed every time, but rather modified according to the action that 
        //     occurred (user connected, user name change, etc.). This will prevent all kinds of weird issues
        //     such as username change input getting canceled when someone joins the room.
        let userList = this.usersArea.userList;
        while (userList.lastChild) {
            userList.removeChild(userList.lastChild);
        }

        for (let i = 0; i < onlineUsers.length; i++) {
            userList.append(onlineUsers[i]);
        }

        for (let i = 0; i < offlineUsers.length; i++) {
            userList.append(offlineUsers[i]);
        }
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

    subscribeToServerEvents() {
        let events = new EventSource("/watch/api/events?token=" + this.token);

        events.addEventListener("welcome", (event) => {
            let connectionId = JSON.parse(event.data);
            console.info("INFO: Received a welcome request with connection id: ", connectionId);
            this.connectionId = connectionId;

            api.setConnectionId(this.connectionId);
        });

        events.addEventListener("usercreate", (event) => {
            let newUser = JSON.parse(event.data)
            this.allUsers.push(newUser)
            console.info("INFO: New user has beed created: ", newUser)
            this.updateUsersArea();
        });

        events.addEventListener("connectionadd", (event) => {
            let userId = JSON.parse(event.data);
            console.info("INFO: New connection added for user id: ", userId)

            for (var i = 0; i < this.allUsers.length; i++) {
                if (this.allUsers[i].id == userId) {
                    this.allUsers[i].connections += 1;
                    break;
                }
            }

            this.updateUsersArea();
        });

        events.addEventListener("connectiondrop", (event) => {
            let userId = JSON.parse(event.data);
            console.info("INFO: Connection dropped for user id: ", userId)

            for (var i = 0; i < this.allUsers.length; i++) {
                if (this.allUsers[i].id == userId) {
                    this.allUsers[i].connections -= 1;
                    break;
                }
            }

            this.updateUsersArea();
        });

        events.addEventListener("playerset", (event) => {
            let response = JSON.parse(event.data);
            console.info("INFO: Received player set event: ", response);

            let entry = response.new_entry;

            let url = entry.url
            if (entry.source_url) {
                url = entry.source_url;
            }

            this.player.setVideoTrack(url);
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
                    if (userId != SERVER_ID) {
                        this.player.setToast("User clicked play.");
                    }
                    this.resyncPlayer(timestamp, userId);
                    this.player.play();
                } break;

                case "pause": {
                    if (userId != SERVER_ID) {
                        this.player.setToast("User clicked pause.");
                    }
                    this.resyncPlayer(timestamp, userId);
                    this.player.pause();
                } break;

                case "seek": {
                    if (userId != SERVER_ID) {
                        this.player.setToast("User seeked.");
                    }
                    this.player.seek(timestamp);
                } break;

                default: {
                    console.error("ERROR: Unknown sync action found", data.action)
                } break;
            }
        });
    }

    async connectToServer() {
        await this.authenticateAccount();
        await this.loadPlayerData();
        await this.loadUsersData();
        // await this.loadPlaylistData();
        this.subscribeToServerEvents();
    }
}

async function main() {
    let room = new Room()
    room.attachPlayerEvents();
    room.attachHtmlEvents();
    await room.connectToServer();
}

main();
