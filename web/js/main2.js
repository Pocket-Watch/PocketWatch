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

        this.rightPanel = {
            tabs: {
                room:     document.getElementById("tab_room"),
                playlist: document.getElementById("tab_playlist"),
                chat:     document.getElementById("tab_chat"),
                history:  document.getElementById("tab_history"),
            },

            content: {
                root:     document.getElementById("right_panel_content"),
                room:     document.getElementById("content_room"),
                playlist: document.getElementById("content_playlist"),
                chat:     document.getElementById("content_chat"),
                history:  document.getElementById("content_history"),
            },
        };

        let content = this.rightPanel.content.room;
        content.style.visibility = "visible";

        let tab = this.rightPanel.tabs.room;
        tab.classList.add("right_panel_tab_selected");

        this.rightPanel.selected = {
            tab:     tab,
            content: content,
        }

        this.proxyEnabled = false;
        this.dropdownIsDown = false;

        /// Current connection id.
        this.connectionId = 0;

        /// Currently connected user. Server User structure.
        this.currentUser = null;

        /// User token string.
        this.token = "";

        /// List of all users in current room.
        this.allUsers = [];

        /// List of all html user elements displayed inside of users_list element.
        this.allUserBoxes = [];

        /// Number of user online.
        this.onlineCount = 0;
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

    attachRightPanelEvents() {
        let select = (tab, content) => {
            this.rightPanel.selected.tab.classList.remove("right_panel_tab_selected");
            this.rightPanel.selected.content.style.visibility = "hidden";

            tab.classList.add("right_panel_tab_selected");
            content.style.visibility = "visible";

            this.rightPanel.selected.tab = tab;
            this.rightPanel.selected.content = content;
        }

        let tabs = this.rightPanel.tabs;
        let content = this.rightPanel.content;

        tabs.room.onclick     = () => select(tabs.room, content.room);
        tabs.playlist.onclick = () => select(tabs.playlist, content.playlist);
        tabs.chat.onclick     = () => select(tabs.chat, content.chat);
        tabs.history.onclick  = () => select(tabs.history, content.history);
    }

    attachHtmlEvents() {
        this.attachRightPanelEvents();

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
        console.log(this.allUsers);

        let onlineBoxes = [];
        let offlineBoxes = [];
        let selfBox = null;

        for (var i = 0; i < this.allUsers.length; i++) {
            let user = this.allUsers[i];
            let userBox = this.createUserBox(user);

            if (user.id == this.currentUser.id) {
                user.online = true;
                this.onlineCount += 1;
                userBox.classList.add("user_box_online");
                selfBox = userBox;
            } else if (user.online) {
                userBox.classList.add("user_box_online");
                onlineBoxes.push(userBox);
            } else {
                userBox.classList.add("user_box_offline");
                offlineBoxes.push(userBox);
            }

            this.allUserBoxes.push(userBox);
        }

        let userList = this.usersArea.userList;
        userList.append(selfBox);

        for (let i = 0; i < onlineBoxes.length; i++) {
            let box = onlineBoxes[i];
            userList.append(box);
        }

        for (let i = 0; i < offlineBoxes.length; i++) {
            let box = offlineBoxes[i];
            userList.append(box);
        }

        this.usersArea.onlineCount.textContent = this.onlineCount;
        this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
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

            if (user.id == this.currentUser.id) {
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

        events.addEventListener("userwelcome", event => {
            let connectionId = JSON.parse(event.data);
            console.info("INFO: Received a welcome request with connection id: ", connectionId);
            this.connectionId = connectionId;

            api.setConnectionId(this.connectionId);
        });

        events.addEventListener("usercreate", event => {
            let user = JSON.parse(event.data)
            this.allUsers.push(user)
            console.info("INFO: New user has beed created: ", user)

            let userBox = this.createUserBox(user);
            userBox.classList.add("user_box_offline");
            this.allUserBoxes.push(userBox);
            this.usersArea.userList.appendChild(userBox);

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userconnected", event => {
            let userId = JSON.parse(event.data);
            console.info("INFO: User connected, ID: ", userId)

            let i = this.allUsers.findIndex(user => user.id == userId);
            this.allUsers[i].online = true;
            this.allUserBoxes[i].classList.remove("user_box_offline");
            this.allUserBoxes[i].classList.add("user_box_online");

            this.onlineCount += 1;

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userdisconnected", event => {
            let userId = JSON.parse(event.data);
            console.info("INFO: User disconnected, ID: ", userId)

            let i = this.allUsers.findIndex(user => user.id == userId);
            this.allUsers[i].online = false;
            this.allUserBoxes[i].classList.remove("user_box_online");
            this.allUserBoxes[i].classList.add("user_box_offline");

            this.onlineCount -= 1;

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("usernameupdate", event => {
            let user = JSON.parse(event.data);
            console.info("INFO: Update user name event for: ", user)

            let i = this.allUsers.findIndex(x => x.id == user.id);
            let input = this.allUserBoxes[i].querySelectorAll('input')[0];
            input.value = user.username;
        });

        events.addEventListener("useravatarupdate", event => {
            let user = JSON.parse(event.data);
            console.info("INFO: Update user avatar event for: ", user)

            let i = this.allUsers.findIndex(x => x.id == user.id);
            let img = document.createElement("img");
            img.src = user.avatar;
            this.allUserBoxes[i].querySelectorAll('img')[0].replaceWith(img);
        });

        events.addEventListener("playerset", event => {
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
