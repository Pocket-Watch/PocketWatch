import {Options, Player} from "./custom_player.js"
import {Playlist} from "./playlist.js"
import {Chat} from "./chat.js"
import {sha256} from "./auth.js"
import * as api from "./api.js";
import {button, div, formatTime, getById, hide, img, show, Storage, svg} from "./util.js";

const SERVER_ID = 0;
const MAX_TITLE_LENGTH = 200;

const LAST_SELECTED_TAB = "last_selected_tab"
const LAST_SELECTED_SUBTITLE = "last_selected_subtitle"

const TAB_ROOM     = 1;
const TAB_PLAYLIST = 2;
const TAB_CHAT     = 3;
const TAB_HISTORY  = 4;

const RECONNECT_AFTER = 5000;

class Room {
    constructor() {
        let video0 = getById("video0");

        let options = new Options();
        options.useAudioGain       = true;
        options.maxVolume          = 1.5;
        options.hideSpeedButton    = true;
        options.hideDownloadButton = true;
        options.hlsConfig.xhrSetup = (xhr, url) => this.configureHlsRequests(xhr, url);
        options.hlsConfig.maxBufferLength = 60;
        this.applyUserOptions(options);

        this.player   = new Player(video0, options);
        this.playlist = new Playlist();
        this.chat     = new Chat();

        this.inputArea = {
            root:          getById("input_area"),
            urlInput:      getById("input_input_box"),
            titleInput:    getById("input_title_input"),
            refererInput:  getById("input_dropdown_referer_input"),
            subtitleInput: getById("input_subtitle_name_input"),
            ytCountInput:  getById("youtube_video_count_input"),

            dropdownButton:       getById("input_dropdown_button"),
            resetButton:          getById("input_reset_button"),
            setButton:            getById("input_set_button"),
            addPlaylistButton:    getById("input_add_playlist_button"),
            selectSubtitleButton: getById("input_select_subtitle_button"),

            youtubeSearchToggle: getById("youtube_search_toggle"),
            asPlaylistToggle:    getById("as_playlist_toggle"),
            addToTopToggle:      getById("add_to_top_toggle"),
            proxyToggle:         getById("proxy_toggle"),

            dropdownContainer: getById("input_dropdown_container"),
        };

        this.usersArea = {
            userList: getById("users_list"),

            onlineCount:  getById("users_online_count"),
            offlineCount: getById("users_offline_count"),
        };

        this.rightPanel = {
            tabs: {
                room:     getById("tab_room"),
                playlist: getById("tab_playlist"),
                chat:     getById("tab_chat"),
                history:  getById("tab_history"),
            },

            content: {
                root:     getById("right_panel_content"),
                room:     getById("content_room"),
                playlist: getById("content_playlist"),
                chat:     getById("content_chat"),
                history:  getById("content_history"),
            },
        };

        this.roomContent = {
            currentUrlInput: getById("room_current_url_input"),
            titleInput:      getById("room_title_input"),
            refererInput:    getById("room_referer_input"),
            uploadSubInput:  getById("room_upload_subtitle_input"),
            uploadFileInput: getById("room_upload_file_input"),
            subsEditInput:   getById("room_subtitle_edit_input"),

            titleUpdateButton: getById("room_title_update_button"),
            uploadSubButton:   getById("room_upload_subtitle_button"),
            uploadFileButton:  getById("room_upload_file_button"),
            copyToInputButton: getById("room_copy_to_input_button"),
            setShiftButton:    getById("room_set_shift_button"),
            subsUpdateButton:  getById("room_subtitle_update_button"),
            subsDeleteButton:  getById("room_subtitle_delete_button"),

            usingProxyCheckbox: getById("room_using_proxy_checkbox"),
            uploadFileProgress: getById("room_upload_file_progress"),
            createdByUsername:  getById("room_created_by_username"),
            createdAtDate:      getById("room_created_at_date"),
            lastActionText:     getById("room_last_action_text"),
            subtitlesSelect:    getById("room_subtitles_select"),
            websiteUptime:      getById("room_website_uptime"),
            websiteVersion:     getById("room_website_version"),
            videoResolution:    getById("room_video_resolution"),

            tokenCopyButton:   getById("room_token_copy_button"),
            tokenSetButton:    getById("room_token_set_button"),
            tokenSetInput:     getById("room_token_set_input"),
        };

        this.connectionLostPopup = getById("connection_lost_popup");

        this.chatNewMessage = getById("tab_chat_new_message_indicator");
        hide(this.chatNewMessage);

        this.newMessageAudio = new Audio('audio/new_message.mp3');

        this.selected_tab     = this.rightPanel.tabs.room;
        this.selected_content = this.rightPanel.content.room;

        this.youtubeSearchEnabled = false;
        this.asPlaylistEnabled    = false;
        this.addToTopEnabled      = false;
        this.proxyEnabled         = false;

        this.roomSelectedSubId = -1;

        // Current connection id.
        this.connectionId = 0;

        // Currently connected user. Server User structure.
        this.currentUserId = -1;

        // User token string.
        this.token = "";

        // List of all users in current room.
        this.allUsers = [];

        // List of all html user elements displayed inside of users_list element.
        this.allUserBoxes = [];

        // Number of user online.
        this.onlineCount = 0;

        // Subtitle file to be attached to the entry.
        this.subtitleFile = null;

        // Id of the currently set entry.
        this.currentEntryId = 0;

        this.currentEntry = {};
    }

    configureHlsRequests(xhr, url) {
        /*if (proxying) {
            xhr.setRequestHeader("Authorization", "Token");
        }*/
    }

    applyUserOptions(options) {
        let alwaysShow = Storage.getBool(Options.ALWAYS_SHOW_CONTROLS);
        if (alwaysShow != null) {
            options.alwaysShowControls = alwaysShow;
        }

        let show = Storage.getBool(Options.SHOW_CONTROLS_ON_PAUSE);
        if (show != null) {
            options.showControlsOnPause = show;
        }
    }

    applyUserPreferences() {
        let last_tab = Storage.getNum(LAST_SELECTED_TAB);
        this.selectRightPanelTab(last_tab);

        let volume = Storage.get("volume");
        if (volume != null) {
            this.player.setVolume(volume);
        }

        let lastSub = Storage.get(LAST_SELECTED_SUBTITLE);
        if (lastSub !== null) {
            this.player.switchSubtitleTrackByUrl(lastSub)
        }

        let subsEnabled = Storage.getBool(Options.SUBTITLES_ENABLED);
        if (subsEnabled) {
            this.player.enableSubtitles();
        }

        let size = Storage.get(Options.SUBTITLE_FONT_SIZE);
        if (size != null) {
            this.player.setSubtitleFontSize(size);
        }

        let position = Storage.getNum(Options.SUBTITLE_VERTICAL_POSITION);
        if (position != null) {
            this.player.setSubtitleVerticalPosition(position);
        }

        let fgColor = Storage.get(Options.SUBTITLE_FOREGROUND_COLOR);
        if (fgColor != null) {
            this.player.setSubtitleForegroundColor(fgColor);
        }

        let fgOpacity = Storage.get(Options.SUBTITLE_FOREGROUND_OPACITY);
        if (fgOpacity != null) {
            this.player.setSubtitleForegroundOpacity(fgOpacity);
        }
        
        let bgColor = Storage.get(Options.SUBTITLE_BACKGROUND_COLOR);
        if (bgColor != null) {
            this.player.setSubtitleBackgroundColor(bgColor);
        }

        let bgOpacity = Storage.get(Options.SUBTITLE_BACKGROUND_OPACITY);
        if (bgOpacity != null) {
            this.player.setSubtitleBackgroundOpacity(bgOpacity);
        }
    }

    attachPlayerEvents() {
        // We have to know if anything is currently playing or whether something is set
        this.player.onControlsPlay(_ => {
            if (this.ended) {
                this.ended = false;
                api.playerPlay(0);
                return;
            }

            if (this.player.getCurrentUrl() === document.baseURI) {
                console.warn("Nothing is set");
                return;
            }

            api.playerPlay(this.player.getCurrentTime());
        });

        this.player.onControlsPause(_ => {
            api.playerPause(this.player.getCurrentTime());
        });

        this.player.onControlsSeeked(timestamp => {
            this.ended = false;
            api.playerSeek(timestamp);
        });

        this.player.onControlsSeeking(timestamp => {
            console.log("User seeking to", timestamp);
        });

        this.player.onControlsNext(_ => {
            api.playerNext(this.currentEntryId);
        });

        this.player.onControlsVolumeSet(volume => {
            // Maybe browsers optimize calls to localStorage and don't write to disk 30 times a second?
            Storage.set("volume", volume)
        });

        this.player.onSettingsChange((key, value) => {
            switch (key) {
                case Options.SHOW_CONTROLS_ON_PAUSE:
                case Options.ALWAYS_SHOW_CONTROLS:
                case Options.SUBTITLES_ENABLED:
                    Storage.setBool(key, value);
                    break;

                case Options.SUBTITLE_FONT_SIZE:
                case Options.SUBTITLE_VERTICAL_POSITION:
                case Options.SUBTITLE_FOREGROUND_COLOR:
                case Options.SUBTITLE_FOREGROUND_OPACITY:
                case Options.SUBTITLE_BACKGROUND_COLOR:
                case Options.SUBTITLE_BACKGROUND_OPACITY:
                    Storage.set(key, value);
                    break;
            }
        });

        this.player.onSubtitleSearch(async search => {
            let jsonResponse = await api.subtitleSearch(search);
            return jsonResponse.ok;
        });

        this.player.onPlaybackEnd(_ => {
            if (this.playlist.autoplayEnabled) {
                api.playerNext(this.currentEntryId);
            } else {
                console.info("Playback ended! Informing the server");
                let endTime = this.player.getDuration();
                if (isNaN(endTime)) {
                    endTime = 0;
                }

                this.ended = true;
                api.playerPause(endTime)
            }
        });

        this.player.onSubtitleSelect(subtitle => {
            Storage.set(LAST_SELECTED_SUBTITLE, subtitle.url);
        });

        this.player.onMetadataLoad(_ => {
            this.roomContent.videoResolution.textContent = this.player.getResolution()
        })

        this.player.onPlaybackError((exception, error) => {
            if (exception.name === "NotAllowedError") {
                this.player.setToast("Auto-play is disabled by your browser!\nClick anywhere on the player to start the playback.");
                return;
            }

            if (exception.name === "AbortError") {
                this.player.setToast("AbortError: Likely the video is slowly loading. Pausing playback!");
                api.playerPause(this.player.getCurrentTime())
                return;
            }

            if (!error) {
                this.player.setToast("UNKNOWN ERROR, press F12 to see what happened!");
                console.error(exception.name + ":", exception.message);
                api.playerPause(this.player.getCurrentTime())
                return;
            }

            if (error.code === MediaError.MEDIA_ERR_DECODE) {
                this.player.setToast("Unable to decode media. " + error.message);
                return;
            }

            if (error.code === MediaError.MEDIA_ERR_SRC_NOT_SUPPORTED) {
                // Distinguish between unsupported codec and 404.
                let errMsg = error.message;
                if (errMsg.startsWith("Failed to init decoder") || errMsg.startsWith("DEMUXER_ERROR_COULD_NOT_OPEN")) {
                    this.player.setToast("Unsupported codec or format: '" + this.player.getCurrentUrl() + "' " + error.message);
                    return;
                }
                if (errMsg.startsWith("NS_ERROR_DOM_INVALID") || errMsg.includes("Empty src")) {
                    this.player.setToast("Nothing is set!");
                    api.playerPause(this.player.getCurrentTime());
                    return;
                }

                if (errMsg.startsWith("404")) {
                    this.player.setToast("Resource not found [404]!");
                } else {
                    this.player.setToast("Unsupported src: '" + this.player.getCurrentUrl() + "' " + error.message);
                }

                api.playerPause(this.player.getCurrentTime())
                return;
            }

        });
    }

    resetInputAreaElements() {
        const input = this.inputArea;

        input.urlInput.value = "";
        input.titleInput.value = "";
        input.refererInput.value = "";
        input.subtitleInput.value = "";
        input.ytCountInput.value = "";

        this.subtitleFile = null;

        this.youtubeSearchEnabled = false;
        input.youtubeSearchToggle.classList.remove("toggle_active");

        this.asPlaylistEnabled = false;
        input.asPlaylistToggle.classList.remove("toggle_active");

        this.addToTopEnabled = false;
        input.addToTopToggle.classList.remove("toggle_active");

        this.proxyEnabled = false;
        input.proxyToggle.classList.remove("toggle_active");
    }

    async createNewRequestEntry() {
        let subtitles = [];
        if (this.subtitleFile) {
            let filename = this.inputArea.subtitleInput.value;
            let sub = await api.subtitleUpload(this.subtitleFile, filename);
            subtitles.push(sub);
        }

        let countString = this.inputArea.ytCountInput.value.trim();
        let count = Number(countString)
        if (!count || count <= 0) {
            count = 20
        }

        const requestEntry = {
            url:          this.inputArea.urlInput.value.trim(),
            title:        this.inputArea.titleInput.value.trim(),
            use_proxy:    this.proxyEnabled,
            referer_url:  this.inputArea.refererInput.value.trim(),
            search_video: this.youtubeSearchEnabled,
            is_playlist:  this.asPlaylistEnabled,
            add_to_top:   this.addToTopEnabled,
            subtitles:    subtitles,
            playlist_skip_count: 0,
            playlist_max_size:   count,
        };

        return requestEntry;
    }

    selectRightPanelTab(tab_type) {
        this.selected_tab.classList.remove("selected");
        this.selected_content.classList.remove("selected");

        let tab     = null;
        let content = null;
        switch (tab_type) {
            case TAB_ROOM: {
                tab     = this.rightPanel.tabs.room;
                content = this.rightPanel.content.room;
            } break;

            case TAB_PLAYLIST: {
                tab     = this.rightPanel.tabs.playlist;
                content = this.rightPanel.content.playlist;
            } break;

            case TAB_CHAT: {
                tab     = this.rightPanel.tabs.chat;
                content = this.rightPanel.content.chat;

                hide(this.chatNewMessage);
            } break;

            case TAB_HISTORY: {
                tab     = this.rightPanel.tabs.history;
                content = this.rightPanel.content.history;
            } break;

            default: {
                tab     = this.rightPanel.tabs.room;
                content = this.rightPanel.content.room;
            } break;
        }

        tab.classList.add("selected");
        content.classList.add("selected");

        this.selected_tab     = tab;
        this.selected_content = content;

        Storage.set(LAST_SELECTED_TAB, tab_type);
    }

    attachRightPanelEvents() {
        let tabs = this.rightPanel.tabs;

        tabs.room.onclick     = _ => this.selectRightPanelTab(TAB_ROOM);
        tabs.playlist.onclick = _ => this.selectRightPanelTab(TAB_PLAYLIST);
        tabs.chat.onclick     = _ => this.selectRightPanelTab(TAB_CHAT);
        tabs.history.onclick  = _ => this.selectRightPanelTab(TAB_HISTORY);


        this.roomContent.titleUpdateButton.onclick = _ => {
            let title = this.roomContent.titleInput.value;
            api.playerUpdateTitle(title);
        };

        this.roomContent.uploadSubButton.onclick = _ => {
            this.roomContent.uploadSubInput.click();
        };

        this.roomContent.uploadSubInput.onchange = async event => {
            let files = event.target.files;

            if (files.length === 0) {
                return;
            }

            let subtitle = await api.subtitleUpload(files[0], files[0].name);
            api.subtitleAttach(subtitle);
        };

        this.roomContent.usingProxyCheckbox.onclick = _ => { return false };

        this.roomContent.uploadFileButton.onclick = _ => {
            this.roomContent.uploadFileProgress.value = "0";
            this.roomContent.uploadFileInput.click();
        };

        this.roomContent.uploadFileInput.onchange = event => {
            let files = event.target.files;

            if (files.length === 0) {
                return;
            }

            let upload = api.uploadMediaWithProgress(files[0], progress => {
                this.roomContent.uploadFileProgress.value = progress;
            });

            upload.then(response => {
                if (response.checkError()) {
                    return;
                }

                this.inputArea.urlInput.value = response.json;
            });
        };

        this.roomContent.copyToInputButton.onclick = _ => {
            this.inputArea.urlInput.value     = this.currentEntry.url;
            this.inputArea.titleInput.value   = this.currentEntry.title;
            this.inputArea.refererInput.value = this.currentEntry.referer_url;

            if (this.currentEntry.use_proxy) {
                this.proxyEnabled = true;
                this.inputArea.proxyToggle.classList.add("widget_toggle_active");
            } else {
                this.proxyEnabled = false;
                this.inputArea.proxyToggle.classList.remove("widget_toggle_active");
            }
        };

        this.roomContent.setShiftButton.onclick = _ => {
            let subs = this.currentEntry.subtitles;
            if (!subs) {
                return;
            }

            for (let i = 0; i < subs.length; i++) {
                let sub = subs[i];
                if (sub.id === this.roomSelectedSubId) {
                    let shift = this.player.getSubtitleShiftByUrl(sub.url);
                    api.subtitleShift(sub.id, shift);
                    break;
                }
            }
        };

        this.roomContent.subtitlesSelect.onchange = event => {
            let id  = Number(event.target.value)
            let sub = this.currentEntry.subtitles.find(sub => sub.id === id);

            this.roomContent.subsEditInput.value = sub.name;
            this.roomSelectedSubId = sub.id;
        };

        this.roomContent.subsUpdateButton.onclick = _ => {
            let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
            if (!subtitle) {
                return
            }

            let newName = this.roomContent.subsEditInput.value.trim();
            api.subtitleUpdate(subtitle.id, newName)
        };

        this.roomContent.subsDeleteButton.onclick = _ => {
            let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
            if (!subtitle) {
                return
            }

            api.subtitleDelete(subtitle.id)
        };

        this.roomContent.tokenCopyButton.onclick = _ => {
            navigator.clipboard.writeText(api.getToken());
        };

        this.roomContent.tokenSetButton.onclick = async _ => {
            let newToken = this.roomContent.tokenSetInput.value
            if (!newToken || newToken === "") {
                console.warn("WARN: Provided token is empty.");
                return;
            }

            let result = await api.userVerify(newToken);
            if (!result.ok) {
                console.warn("WARN: User with provided token does not exist.");
                return;
            }

            let currToken = api.getToken();
            await api.userDelete(currToken);

            Storage.set("token", newToken);
            window.location.reload();
        };
    }

    attachUrlAreaEvents() {
        this.inputArea.dropdownButton.onclick = _ => {
            this.inputArea.root.classList.toggle("input_area_expand");
        }

        this.inputArea.resetButton.onclick = _ => {
            this.resetInputAreaElements();
        }

        this.inputArea.setButton.onclick = async _ => {
            let entry = await this.createNewRequestEntry();
            api.playerSet(entry).then(jsonResponse => {
                if (jsonResponse.checkError()) {
                    return;
                }

                // Only reset if request was successful
                this.resetInputAreaElements();
            });
        }

        this.inputArea.addPlaylistButton.onclick = async _ => {
            let entry = await this.createNewRequestEntry();
            if (entry.url) {
                api.playlistAdd(entry);
                this.resetInputAreaElements();
            }
        }

        this.inputArea.selectSubtitleButton.onclick = _ => {
            let input = document.createElement('input');
            input.type = "file";
            input.accept = ".srt,.vtt";
            input.onchange = event => {
                let files = event.target.files;

                if (files.length === 0) {
                    return;
                }

                console.log("File selected: ", files[0]);
                this.subtitleFile = files[0];
                this.inputArea.subtitleInput.value = this.subtitleFile.name;
            }
            input.click();
        }

        this.inputArea.youtubeSearchToggle.onclick = _ => {
            this.inputArea.youtubeSearchToggle.classList.toggle("widget_toggle_active");
            this.youtubeSearchEnabled = !this.youtubeSearchEnabled;
        }

        this.inputArea.asPlaylistToggle.onclick = _ => {
            this.inputArea.asPlaylistToggle.classList.toggle("widget_toggle_active");
            this.asPlaylistEnabled = !this.asPlaylistEnabled;
        }

        this.inputArea.addToTopToggle.onclick = _ => {
            this.inputArea.addToTopToggle.classList.toggle("widget_toggle_active");
            this.addToTopEnabled = !this.addToTopEnabled;
        }

        this.inputArea.proxyToggle.onclick = _ => {
            this.inputArea.proxyToggle.classList.toggle("widget_toggle_active");
            this.proxyEnabled = !this.proxyEnabled;
        }
    }

    attachHtmlEvents() {
        this.playlist.attachPlaylistEvents();
        this.attachUrlAreaEvents();
        this.attachRightPanelEvents();
    }

    getUsernameByUserId(userId) {
        if (userId === SERVER_ID) {
            return "Server";
        }

        let index = this.allUsers.findIndex(user => user.id === userId);
        return index === -1 ? userId : this.allUsers[index].username;
    }

    async createNewAccount() {
        this.token = await api.userCreate();
        api.setToken(this.token);
        Storage.set("token", this.token);
    }

    async authenticateAccount(firstTry) {
        this.token = Storage.get("token");
        api.setToken(this.token);

        let verification = await api.userVerify(this.token);
        if (firstTry && !verification.ok) {
            return false;
        }

        if (verification.checkError()) {
            return false;
        }

        this.currentUserId = verification.json;
        return true;
    }

    async loadPlayerData() {
        let state = await api.playerGet();
        this.playlist.setAutoplay(state.player.autoplay);
        this.playlist.setLooping(state.player.looping);

        let entry = state.entry;
        this.setEntryEvent(entry);
        this.player.seek(state.player.timestamp)
    }

    clearUsersArea() {
        this.onlineCount = 0;
        this.allUserBoxes = [];

        const userList = this.usersArea.userList;
        while (userList.lastChild) {
            userList.removeChild(userList.lastChild);
        }
    }

    updateUsersArea() {
        const userList = this.usersArea.userList;

        let onlineBoxes = [];
        let offlineBoxes = [];
        let selfBox = null;

        for (var i = 0; i < this.allUsers.length; i++) {
            let user = this.allUsers[i];
            let userBox = this.createUserBox(user);

            if (user.id === this.currentUserId) {
                selfBox = userBox;
            } else if (user.online) {
                onlineBoxes.push(userBox);
            } else {
                offlineBoxes.push(userBox);
            }

            if (user.online) {
                this.onlineCount += 1;
                userBox.classList.add("user_box_online");
            } else {
                userBox.classList.add("user_box_offline");
            }

            this.allUserBoxes.push(userBox);
        }

        userList.append(selfBox);

        for (let i = 0; i < onlineBoxes.length; i++) {
            let box = onlineBoxes[i];
            userList.append(box);
        }

        for (let i = 0; i < offlineBoxes.length; i++) {
            let box = offlineBoxes[i];
            userList.append(box);
        }

        this.usersArea.onlineCount.textContent  = this.onlineCount;
        this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
    }

    markAllUsersOffline() {
        for (let i = 0; i < this.allUsers.length; i++) {
            this.allUsers[i].online = false;
        }
    }

    async loadUsersData() {
        this.allUsers = await api.userGetAll();
        console.log(this.allUsers);

        this.clearUsersArea();
        this.updateUsersArea();
    }

    createUserBox(user) {
        let userBox       = div("user_box");
        let userBoxTop    = div("user_box_top");
        let userAvatar    = img(user.avatar);
        let userBoxBottom = div("user_box_bottom");
        let usernameInput = document.createElement("input");

        //
        // Configuring parameters for html elements.
        //
        usernameInput.readOnly = true;
        usernameInput.value = user.username;

        //
        // Constructing html element structure.
        //
        userBox.append(userBoxTop); {
            userBoxTop.append(userAvatar);
        }
        userBox.append(userBoxBottom); {
            userBoxBottom.append(usernameInput);
        }

        if (user.id == this.currentUserId) {
            // NOTE(kihau): Temporary. The user box CSS styling and code logic will be slightly refactored.
            // userBox.style.borderColor    = "#ebdbb2";
            // userBoxTop.style.borderColor = "#ebdbb2";

            // userBox.style.borderColor    = "#b8bb26";
            // userBoxTop.style.borderColor = "#b8bb26";

            userBox.style.borderColor    = "#d5c4a1";
            userBoxTop.style.borderColor = "#d5c4a1";
            userBox.style.boxShadow      = "0px 0px 4px #fbf1cf inset";


            let changeAvatarButton = button("user_box_change_avatar", "Update your avatar");
            let editNameButton = button("user_box_edit_name_button", "Change your username");
            let editSvg = svg("svg/main_icons.svg#edit2");

            //
            // Configuring parameters for html elements.
            //
            changeAvatarButton.textContent = "E";

            //
            // Attaching events to html elements
            //
            changeAvatarButton.onclick = _ => {
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

            usernameInput.addEventListener("focusout", _ => {
                usernameInput.readOnly = true;

                let user = this.allUsers.find(user => this.currentUserId === user.id);
                let newUsername = usernameInput.value.trim()
                if (newUsername && newUsername !== user.username) {
                    api.userUpdateName(newUsername);
                } else {
                    usernameInput.value = user.username
                }
            });

            usernameInput.addEventListener("keypress", event => {
                if (event.key === "Enter") {
                    usernameInput.readOnly = true;

                    let user = this.allUsers.find(user => this.currentUserId === user.id);
                    let newUsername = usernameInput.value.trim()
                    if (newUsername && newUsername !== user.username) {
                        api.userUpdateName(newUsername);
                    } else {
                        usernameInput.value = user.username
                    }
                }
            });

            editNameButton.onclick = () => {
                usernameInput.readOnly = false;
                usernameInput.focus();
            };

            //
            // Constructing html element structure
            //
            userBoxTop.append(changeAvatarButton);
            userBoxBottom.append(editNameButton); {
                editNameButton.append(editSvg);
            }
        }

        return userBox;
    }


    async loadPlaylistData() {
        let entries = await api.playlistGet();
        if (!entries) {
            return;
        }

        console.log(entries);

        this.playlist.clear();
        // TODO(kihau): Performance problem when number of entries is large. Needs to be fixed at some point.
        this.playlist.loadEntries(entries, this.allUsers);
    }

    async loadChatData() {
        let messages = await api.chatGet();
        if (!messages) {
            return;
        }

        console.log(messages);
        this.chat.clear();
        this.chat.loadMessages(messages, this.allUsers);
    }

    updateRoomContent(entry) {
        this.roomContent.currentUrlInput.value         = entry.url;
        this.roomContent.titleInput.value              = entry.title;
        this.roomContent.usingProxyCheckbox.checked    = entry.use_proxy;
        this.roomContent.refererInput.value            = entry.referer_url;
        this.roomContent.createdByUsername.textContent = this.getUsernameByUserId(entry.user_id);
        this.roomContent.createdAtDate.textContent     = new Date(entry.created);

        this.updateRoomSubtitlesHtml(entry);
    }

    setEntryEvent(entry) {
        this.updateRoomContent(entry);

        this.currentEntry            = entry;
        this.currentEntryId          = entry.id;
        this.playlist.currentEntryId = entry.id;

        let url = entry.url
        if (!url) {
            this.setNothing();
            return;
        }

        if (entry.source_url) {
            url = entry.source_url;
        }

        this.player.setVideoTrack(url);

        if (entry.title) {
            if (entry.title.length > MAX_TITLE_LENGTH) {
                entry.title = entry.title.substring(0, MAX_TITLE_LENGTH);
            }
            this.player.setTitle(entry.title);
        }

        if (entry.subtitles && entry.subtitles.length > 0) {
            // Clear subtitles if there's a replacement
            this.player.clearAllSubtitleTracks();
            for (let i = 0; i < entry.subtitles.length; i++) {
                let sub = entry.subtitles[i];
                this.player.addSubtitle(sub.url, sub.name, sub.shift);
            }
        }

        this.player.setPoster(null)
        if (entry.thumbnail) {
            this.player.setPoster(entry.thumbnail);
        }

        if (this.playlist.autoplayEnabled) {
            this.player.play();
        }
    }

    setNothing() {
        Storage.remove(LAST_SELECTED_SUBTITLE);

        this.player.discardPlayback();
        this.player.setTitle(null);
        this.player.setPoster("")
        this.player.setToast("Nothing is playing at the moment!");
        this.player.clearAllSubtitleTracks();
    }

    resyncPlayer(timestamp, userId) {
        const MAX_DESYNC = 1.5;
        let desync = timestamp - this.player.getCurrentTime();

        if (userId == 0) {
            console.info("INFO: Received resync event from SERVER at", timestamp, "with desync:", desync);
        } else {
            console.info("INFO: Received resync event from USER id", userId, "at", timestamp, "with desync:", desync);
        }

        if (Math.abs(desync) > MAX_DESYNC && !this.player.isLive()) {
            let diff = Math.abs(desync) - MAX_DESYNC
            console.warn("You are desynced! MAX_DESYNC(" + MAX_DESYNC + ") exceeded by:", diff, "Trying to resync now!");
            this.player.seek(timestamp);
        }
    }

    async login(login, password) {
        let passwordHash = await sha256(password);
        console.log(passwordHash);
        // Send
    }


    listenToServerEvents() {
        let events = new EventSource("/watch/api/events?token=" + this.token);
        events.onopen = async _ => {
            console.info("INFO: Connection to events opened.");

            this.connectionLostPopup.classList.remove("show")

            await this.loadUsersData();
            await this.loadPlayerData();
            await this.loadPlaylistData();
            await this.loadChatData();
            api.uptime().then(uptime   => this.roomContent.websiteUptime.textContent  = uptime);
            api.version().then(version => this.roomContent.websiteVersion.textContent = version);
        };

        events.onerror = _ => {
            events.close()
            console.error("ERROR: Connection to the server was lost. Attempting to reconnect in", RECONNECT_AFTER, "ms");
            this.handleDisconnect();
        };

        this.subscribeToServerEvents(events);
    }

    updateRoomSubtitlesHtml(entry) {
        let select = this.roomContent.subtitlesSelect;
        while (select.lastChild) {
            select.removeChild(select.lastChild);
        }

        let subs = entry.subtitles;
        if (!subs || subs.length === 0) {
            this.roomSelectedSubId = -1;
            this.roomContent.subsEditInput.value = "";
            return;
        }

        this.roomContent.subsEditInput.value = subs[0].name;
        this.roomSelectedSubId = subs[0].id;

        for (let i = 0; i < subs.length; i++) {
            let sub = subs[i];
            let option = document.createElement("option");

            option.textContent = sub.name;
            option.value       = sub.id;
            select.appendChild(option);
        }
    }

    subscribeToSubtitleEvents(events) {
        events.addEventListener("subtitledelete", event => {
            if (!event.data) {
                console.warn("Subtitle delete event failed, event data is null.")
                return;
            }

            let subId = JSON.parse(event.data);
            console.info("Received subtitle delete event for subtitle with ID:", subId);

            let subs = this.currentEntry.subtitles;
            if (!subs) {
                console.warn("Subtitle delete event failed, currentEntry subtitles is null.")
                return;
            }

            let index = subs.findIndex(sub => sub.id === subId);
            if (index === -1) {
                console.warn("Subtitle delete event failed, subtitle index is -1.")
                return;
            }

            subs.splice(index, 1);

            this.player.clearAllSubtitleTracks();
            for (let i = 0; i < subs.length; i++) {
                const sub = subs[i];
                this.player.addSubtitle(sub.url, sub.name, sub.shift);
            }

            this.updateRoomSubtitlesHtml(this.currentEntry);
        });

        events.addEventListener("subtitleupdate", event => {
            let data = JSON.parse(event.data);
            if (!data) {
                console.warn("Subtitle update event failed, event data is null.")
                return;
            }

            console.info("Received subtitle update event with:", data);

            let subs = this.currentEntry.subtitles;
            if (!subs) {
                console.warn("Subtitle update event failed, currentEntry subtitles is null.")
                return;
            }

            let index = subs.findIndex(sub => sub.id === data.id);
            if (index === -1) {
                console.warn("Subtitle update event failed, subtitle index is -1.")
                return;
            }

            subs[index].name = data.name;

            this.player.clearAllSubtitleTracks();
            for (let i = 0; i < subs.length; i++) {
                const sub = subs[i];
                this.player.addSubtitle(sub.url, sub.name, sub.shift);
            }

            this.updateRoomSubtitlesHtml(this.currentEntry);
        });

        events.addEventListener("subtitleattach", event => {
            let subtitle = JSON.parse(event.data);
            console.log(subtitle);
            this.player.addSubtitle(subtitle.url, subtitle.name, subtitle.shift);
            this.player.setToast("Subtitle added: " + subtitle.name);

            if (!this.currentEntry.subtitles) {
                this.currentEntry.subtitles = [];
            }

            this.currentEntry.subtitles.push(subtitle);
            this.updateRoomSubtitlesHtml(this.currentEntry);
        });

        events.addEventListener("subtitleshift", event => {
            let data = JSON.parse(event.data);

            let subs = this.currentEntry.subtitles;
            for (let i = 0; i < subs.length; i++) {
                let sub = subs[i];
                if (sub.id === data.id) {
                    this.player.setSubtitleShiftByUrl(sub.url, data.shift);
                    subs[i].shift = data.shift;
                    break;
                }
            }
        });
    }

    subscribeToServerEvents(events) {
        this.subscribeToSubtitleEvents(events);

        events.addEventListener("userwelcome", event => {
            let connectionId = JSON.parse(event.data);
            console.info("INFO: Received a welcome request with connection id:", connectionId);
            this.connectionId = connectionId;

            api.setConnectionId(this.connectionId);
        });

        events.addEventListener("usercreate", event => {
            let user = JSON.parse(event.data)
            this.allUsers.push(user)
            console.info("INFO: New user has been created: ", user)

            let userBox = this.createUserBox(user);
            userBox.classList.add("user_box_offline");
            this.allUserBoxes.push(userBox);
            this.usersArea.userList.appendChild(userBox);

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userdelete", event => {
            let target = JSON.parse(event.data)
            let index = this.allUsers.findIndex(user => user.id === target.id);
            let user = this.allUsers.splice(index, 1)[0];
            let userBox = this.allUserBoxes.splice(index, 1)[0];

            console.info("Removing user:", user, "with its user box", userBox);
            this.usersArea.userList.removeChild(userBox);
            
            this.onlineCount -= 1;

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        // All user-related update events can be multiplexed into one "user-update" event to simplify logic
        // The server will always serve the up-to-date snapshot of User which should never exceed 1 kB in practice
        events.addEventListener("userconnected", event => {
            let userId = JSON.parse(event.data);
            console.info("INFO: User connected, ID: ", userId)

            let userBoxes = this.usersArea.userList;
            let onlineBoxes = userBoxes.getElementsByClassName("user_box_online");
            let lastOnlineBox = onlineBoxes[onlineBoxes.length - 1];

            let i = this.allUsers.findIndex(user => user.id === userId);
            this.allUsers[i].online = true;

            this.allUserBoxes[i].classList.remove("user_box_offline");
            this.allUserBoxes[i].classList.add("user_box_online");

            let connectedNow = this.allUserBoxes[i];
            if (lastOnlineBox) {
                userBoxes.insertBefore(connectedNow, lastOnlineBox.nextSibling);
            } else {
                userBoxes.appendChild(connectedNow);
            }

            this.onlineCount += 1;

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userdisconnected", event => {
            let userId = JSON.parse(event.data);
            console.info("INFO: User disconnected, ID: ", userId)

            let userBoxes = this.usersArea.userList;
            let offlineBoxes = userBoxes.getElementsByClassName("user_box_offline");
            let firstOfflineBox = offlineBoxes[0];

            let i = this.allUsers.findIndex(user => user.id === userId);
            this.allUsers[i].online = false;

            this.allUserBoxes[i].classList.remove("user_box_online");
            this.allUserBoxes[i].classList.add("user_box_offline");

            let disconnectedNow = this.allUserBoxes[i];
            if (firstOfflineBox) {
                userBoxes.insertBefore(disconnectedNow, firstOfflineBox);
            } else {
                userBoxes.appendChild(disconnectedNow);
            }

            this.onlineCount -= 1;

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userupdate", event => {
            let user = JSON.parse(event.data);
            console.info("INFO: Update user name event for: ", user)

            let i = this.allUsers.findIndex(x => x.id == user.id);
            this.allUsers[i] = user; // emplace the user

            let input = this.allUserBoxes[i].querySelectorAll('input')[0];
            input.value = user.username;

            let img = document.createElement("img");
            img.src = user.avatar;
            this.allUserBoxes[i].querySelectorAll('img')[0].replaceWith(img);

            this.playlist.handleUserUpdate(user);
        });

        events.addEventListener("playerset", event => {
            let response = JSON.parse(event.data);
            console.info("INFO: Received player set event: ", response);

            let newEntry = response.new_entry;
            this.setEntryEvent(newEntry);

            let prevEntry = response.prev_entry;
            if (this.playlist.loopingEnabled && prevEntry.url !== "") {
                this.playlist.addEntry(prevEntry, this.allUsers);
            }
        });

        events.addEventListener("playernext", event => {
            let response = JSON.parse(event.data);
            let prevEntry = response.prev_entry;

            let newEntry = response.new_entry;
            this.setEntryEvent(newEntry);

            if (this.playlist.entries.length !== 0) {
                if (this.playlist.loopingEnabled && prevEntry.url !== "") {
                    this.playlist.addEntry(prevEntry, this.allUsers);
                }

                this.playlist.removeAt(0);
            }
        });

        events.addEventListener("playerlooping", event => {
            let looping = JSON.parse(event.data);
            this.playlist.setLooping(looping)
        });

        events.addEventListener("playerautoplay", event => {
            let autoplay = JSON.parse(event.data);
            this.playlist.setAutoplay(autoplay)
        });

        events.addEventListener("playerupdatetitle", event => {
            let title = JSON.parse(event.data);
            this.player.setTitle(title);
            this.roomContent.titleInput.value = title;
        });

        events.addEventListener("sync", event => {
            let data = JSON.parse(event.data);
            if (!data) {
                console.error("ERROR: Failed to parse event data")
                return;
            }

            let timestamp = data.timestamp;
            let userId = data.user_id;
            let username = this.getUsernameByUserId(userId);

            switch (data.action) {
                case "play": {
                    if (userId != SERVER_ID) {
                        this.player.setToast(username + " clicked play.");
                        this.roomContent.lastActionText.textContent = username + " clicked play.";
                    }

                    this.resyncPlayer(timestamp, userId);
                    this.player.play();
                } break;

                case "pause": {
                    if (userId != SERVER_ID) {
                        this.player.setToast(username + " clicked pause.");
                        this.roomContent.lastActionText.textContent = username + " clicked pause.";
                    }

                    this.resyncPlayer(timestamp, userId);
                    this.player.pause();
                } break;

                case "seek": {
                    if (userId != SERVER_ID) {
                        let time = formatTime(timestamp);
                        this.player.setToast(username + " seeked to " + time);
                        this.roomContent.lastActionText.textContent = username + " seeked to " + time;
                    }

                    if (!this.player.isLive()) {
                        this.player.seek(timestamp);
                    }
                } break;

                default: {
                    console.error("ERROR: Unknown sync action found", data.action)
                } break;
            }
        });

        events.addEventListener("playlist", event => {
            let response = JSON.parse(event.data);
            console.info("INFO: Received playlist event for:", response.action, "with:", response.data);
            this.playlist.handleServerEvent(response.action, response.data, this.allUsers);
        });

        events.addEventListener("messagecreate", event => {
            let data = JSON.parse(event.data);
            console.info("INFO: New message received from server");

            if (this.selected_tab !== this.rightPanel.tabs.chat) {
                show(this.chatNewMessage);
            }

            if (this.selected_tab !== this.rightPanel.tabs.chat || this.player.isFullscreen()) {
                this.newMessageAudio.play();
            }

            this.chat.addMessage(data, this.allUsers);
        });
    }

    handleDisconnect() {
        this.markAllUsersOffline();
        this.clearUsersArea();
        this.updateUsersArea();
        this.connectionLostPopup.classList.add("show")
        setTimeout(_ => this.connectToServer(), RECONNECT_AFTER);
    }

    async connectToServer() {
        try {
            // Temporary workaround for lack of persistent server-side account storage
            if (!await this.authenticateAccount(true)) {
                await this.createNewAccount();
                await this.authenticateAccount();
            }
        } catch (_) {}

        this.listenToServerEvents();
    }
}

async function main() {
    let room = new Room();
    room.attachPlayerEvents();
    room.attachHtmlEvents();
    await room.connectToServer();
    room.applyUserPreferences();

    // Expose room to browser console for debugging.
    window.room = room;
}

main();
