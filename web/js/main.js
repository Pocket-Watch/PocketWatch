import { Options, Player } from "./custom_player.js"
import { Playlist } from "./playlist.js"
import { History } from "./history.js"
import { Chat } from "./chat.js"
import { sha256 } from "./auth.js"
import * as api from "./api.js";
import { Storage, button, div, formatTime, formatByteCount, getById, dynamicImg, svg, show, hide, fileInput } from "./util.js";

const SERVER_ID = 0;

const SELECTED_THEME         = "selected_theme"
const USER_AVATAR_ANIMATIONS = "user_avatar_animations"
const NEW_MESSAGE_SOUND      = "new_message_sound"
const ROOM_THEATER_MODE      = "room_theader_mode"
const LAST_SELECTED_TAB      = "last_selected_tab"
const LAST_SELECTED_SUBTITLE = "last_selected_subtitle"

const TAB_ROOM     = 1;
const TAB_PLAYLIST = 2;
const TAB_CHAT     = 3;
const TAB_HISTORY  = 4;

const RECONNECT_AFTER = 1500;
const MAX_CHAT_LOAD = 100;

class Room {
    constructor() {
        let video0 = getById("video0");

        let options = new Options();
        options.useAudioGain       = true;
        options.maxVolume          = 1.5;
        options.hideSpeedButton    = true;
        options.hideDownloadButton = true;
        options.hlsConfig.xhrSetup = (xhr, url) => this.configureHlsRequests(xhr, url);
        options.hlsConfig.maxBufferLength = 40;
        options.hlsConfig.maxBufferSize = 30 * 1000 * 1000;
        this.applyPlayerOptions(options);

        this.player   = new Player(video0, options);
        this.playlist = new Playlist();
        this.chat     = new Chat();
        this.history  = new History();

        this.pageRoot      = getById("page_root");
        this.selectedTheme = getById("selected_theme");

        this.settingsMenu = {
            modal:       getById("settings_menu_modal"),
            root:        getById("settings_menu_root"),
            closeButton: getById("settings_menu_close_button"),

            websiteUptime:   getById("settings_website_uptime"),
            websiteVersion:  getById("settings_website_version"),
            tokenCopyButton: getById("settings_token_copy_button"),
            tokenSetButton:  getById("settings_token_set_button"),
            tokenSetInput:   getById("settings_token_set_input"),

            animatedAvatarsToggle:   getById("animated_avatars_toggle"),
            newMessageSoundToggle:   getById("new_message_sound_toggle"),
            theaterModeToggle:       getById("theater_mode_toggle"),
            themeSwitcherSelect:     getById("settings_switch_theme"),
            deleteYourAccountButton: getById("delete_your_account"),
            confirmAccountDelete:    getById("confirm_deletion_phrase"),
        };

        this.connectionLostPopup = getById("connection_lost_popup");

        this.entryArea = {
            root:              getById("entry_area"),
            dropdownContainer: getById("entry_dropdown_container"),

            // Top Controls 
            dropdownButton:    getById("entry_dropdown_button"),
            resetButton:       getById("entry_reset_button"),
            urlInput:          getById("entry_url_input"),
            urlLabel:          getById("entry_url_label"),
            setButton:         getById("entry_set_button"),
            addPlaylistButton: getById("entry_add_playlist_button"),

            // General
            titleInput:     getById("entry_title_input"),
            addToTopToggle: getById("entry_add_to_top_toggle"),
            proxyToggle:    getById("entry_proxy_toggle"),
            refererInput:   getById("entry_dropdown_referer_input"),

            // Subtitles
            selectSubtitleButton:   getById("entry_select_subtitle_button"),
            subtitleNameInput:      getById("entry_subtitle_name_input"),
            subtitleUrlInput:       getById("entry_subtitle_url_input"),
            subtitleReattachToggle: getById("entry_subtitle_reattach_toggle"),

            // Youtube
            youtubeSearchToggle:   getById("entry_youtube_search_toggle"),
            youtubePlaylistToggle: getById("entry_youtube_playlist_toggle"),
            ytCountInput:          getById("youtube_video_count_input"),
            ytSkipCountInput:      getById("youtube_video_skip_count_input"),
        };

        this.usersArea = {
            userList: getById("users_list"),

            onlineCount:  getById("users_online_count"),
            offlineCount: getById("users_offline_count"),

            settingsButton: getById("users_settings_button"),
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
            urlInput:        getById("room_entry_url_input"),
            titleInput:      getById("room_entry_title_input"),
            refererInput:    getById("room_entry_referer_input"),
            uploadSubInput:  getById("room_upload_subtitle_input"),
            subsEditInput:   getById("room_subtitle_edit_input"),

            titleUpdateButton:  getById("room_title_update_button"),
            uploadSubButton:    getById("room_upload_subtitle_button"),
            copyEntryButton:    getById("room_copy_entry_button"),
            setShiftButton:     getById("room_set_shift_button"),
            subsUpdateButton:   getById("room_subtitle_update_button"),
            subsDeleteButton:   getById("room_subtitle_delete_button"),
            openSettingsButton: getById("room_open_settings_button"),

            usingProxyCheckbox: getById("room_entry_proxy_enabled"),
            uploadFileProgress: getById("room_upload_file_progress"),
            createdByUsername:  getById("room_created_by_username"),
            lastActionText:     getById("room_last_action_text"),
            subtitlesSelect:    getById("room_subtitles_select"),
            videoResolution:    getById("room_video_resolution"),
            createdAtDate:      getById("room_created_at_date"),

            upload: {
                placeholderRoot: getById("room_upload_media_placeholder"),
                progressRoot:    getById("room_upload_media_progress"),
                filepicker:      getById("room_upload_media_filepicker"),
                percent:         getById("room_upload_media_progress_percent"),
                barCurrent:      getById("room_upload_media_progress_bar_current"),
                uploaded:        getById("room_upload_media_progress_uploaded"),
                transfer:        getById("room_upload_media_progress_transfer"),

            },

            browse: {
                videoButton:     getById("room_media_browse_button_video"),
                audioButton:     getById("room_media_browse_button_audio"),
                subtitlesButton: getById("room_media_browse_button_subtitles"),
                imagesButton:    getById("room_media_browse_button_images"),
            }
        };

        this.chatNewMessage  = getById("tab_chat_new_message_indicator");
        this.newMessageAudio = new Audio("audio/new_message.mp3");

        this.selected_tab     = this.rightPanel.tabs.room;
        this.selected_content = this.rightPanel.content.room;

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

    showSettingsMenu(_settingsTab) {
        show(this.settingsMenu.modal);
        this.settingsMenu.root.focus();
    }

    hideSettingsMenu() {
        hide(this.settingsMenu.modal);
    }

    configureHlsRequests(xhr, url) {
        /*if (proxying) {
            xhr.setRequestHeader("Authorization", "Token");
        }*/
    }

    applyPlayerOptions(options) {
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
        // Room settings
        let last_tab = Storage.getNum(LAST_SELECTED_TAB);
        this.selectRightPanelTab(last_tab);

        let animation = Storage.get(USER_AVATAR_ANIMATIONS);
        if (!animation || Storage.isTrue(animation)) {
            this.settingsMenu.animatedAvatarsToggle.classList.add("active");
            this.pageRoot.classList.remove("disable_image_animations");
        } else {
            this.settingsMenu.animatedAvatarsToggle.classList.remove("active");
            this.pageRoot.classList.add("disable_image_animations");
        }

        let newMessageSound = Storage.get(NEW_MESSAGE_SOUND);
        if (!newMessageSound || Storage.isTrue(newMessageSound)) {
            this.settingsMenu.newMessageSoundToggle.classList.add("active");
        } else {
            this.settingsMenu.newMessageSoundToggle.classList.remove("active");
        }

        let theater = Storage.get(ROOM_THEATER_MODE);
        if (theater && Storage.isTrue(theater)) {
            this.pageRoot.classList.add("theater_mode");
            this.settingsMenu.theaterModeToggle.classList.add("active");
        } else {
            this.pageRoot.classList.remove("theater_mode");
            this.settingsMenu.theaterModeToggle.classList.remove("active");
        }

        let theme = Storage.get(SELECTED_THEME);
        if (theme) {
            this.selectedTheme.href = `css/themes/${theme}.css`
            this.settingsMenu.themeSwitcherSelect.value = theme;
        } 

        // Player settings
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
        let fgOpacity = Storage.get(Options.SUBTITLE_FOREGROUND_OPACITY);
        if (fgColor != null && fgOpacity != null) {
            this.player.setSubtitleForegroundColor(fgColor, fgOpacity);
        }
        
        let bgColor = Storage.get(Options.SUBTITLE_BACKGROUND_COLOR);
        let bgOpacity = Storage.get(Options.SUBTITLE_BACKGROUND_OPACITY);
        if (bgColor != null && bgOpacity != null) {
            this.player.setSubtitleBackgroundColor(bgColor, bgOpacity);
        }

        let videoFit = Storage.get(Options.VIDEO_FIT);
        if (videoFit === "fit") {
            this.player.fitVideoToScreen();
        } else if (videoFit === "stretch") {
            this.player.stretchVideoToScreen();
        }
    }

    attachPlayerEvents() {
        this.player.onControlsPlay(_ => {
            if (!this.player.getCurrentUrl()) {
                return;
            }

            if (this.player.getCurrentTime() >= this.player.getDuration()) {
                api.playerPlay(0);
            } else {
                api.playerPlay(this.player.getCurrentTime());
            }
        });

        this.player.onControlsPause(_ => {
            api.playerPause(this.player.getCurrentTime());
        });

        this.player.onControlsSeeked(timestamp => {
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

                case Options.VIDEO_FIT:
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

                api.playerPause(endTime);
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

                api.playerPause(this.player.getCurrentTime());
                return;
            }

        });

        // NOTE(kihau): This is a hack to fix autoplay issue with HLS sources.
        this.player.onDataLoad(_ => {
            if (this.playlist.autoplayEnabled) {
                this.player.play();
            }
        });
    }

    resetEntryAreaElements() {
        const entry = this.entryArea;

        entry.urlInput.value = "";
        entry.titleInput.value = "";
        entry.refererInput.value = "";
        entry.subtitleNameInput.value = "";
        entry.subtitleUrlInput.value = "";
        entry.ytCountInput.value = "";
        entry.ytSkipCountInput.value = "";

        this.subtitleFile = null;

        entry.youtubeSearchToggle.classList.remove("active");
        entry.youtubePlaylistToggle.classList.remove("active");
        entry.addToTopToggle.classList.remove("active");
        entry.proxyToggle.classList.remove("active");
        entry.subtitleReattachToggle.classList.remove("active");

        entry.urlLabel.textContent = "Entry URL";
    }

    async createNewRequestEntry() {
        let area = this.entryArea;

        let subname = area.subtitleNameInput.value.trim();
        let suburl  = area.subtitleUrlInput.value.trim();

        let subtitles = [];

        let sub;
        if (this.subtitleFile) {
            sub = await api.subtitleUpload(this.subtitleFile, subname);
        } else if (suburl) {
            let response = await api.subtitleDownload(suburl, subname);
            sub = response.json;
        }

        if (sub) {
            subtitles.push(sub);
        }

        if (area.subtitleReattachToggle.classList.contains("active")) {
            subtitles = subtitles.concat(this.currentEntry.subtitles);
        }

        let countString = area.ytCountInput.value.trim();
        let count = Number(countString)
        if (!count || count <= 0) {
            count = 20
        }

        let skipCountString = area.ytSkipCountInput.value.trim();
        let skipCount = Number(skipCountString)
        if (!skipCount || skipCount <= 0) {
            skipCount = 0
        }

        const requestEntry = {
            url:          area.urlInput.value.trim(),
            title:        area.titleInput.value.trim(),
            referer_url:  area.refererInput.value.trim(),
            use_proxy:    area.proxyToggle.classList.contains("active"),
            search_video: area.youtubeSearchToggle.classList.contains("active"),
            is_playlist:  area.youtubePlaylistToggle.classList.contains("active"),
            add_to_top:   area.addToTopToggle.classList.contains("active"),
            subtitles:    subtitles,
            playlist_skip_count: skipCount,
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

        if (tab_type === TAB_CHAT) {
            hide(this.chatNewMessage);
            this.chat.keepAtBottom();
        }

        Storage.set(LAST_SELECTED_TAB, tab_type);
    }

    startMediaFileUpload(file) {
        const room = this.roomContent;

        let timeRate = new Date().getTime();
        // Byte size accumulated after each second.
        let accumulated = 0;

        let bytesPrev = 0;

        room.upload.percent.textContent = "0%";
        room.upload.barCurrent.style.width = "0%";
        room.upload.uploaded.textContent = "0 B / 0 B";
        room.upload.transfer.textContent = "0 B/s";

        room.upload.placeholderRoot.classList.add("hide");
        room.upload.progressRoot.classList.remove("hide");

        let upload = api.uploadMediaWithProgress(file, event => {
            let progress = (event.loaded / event.total) * 100;

            room.upload.percent.textContent = Math.floor(progress) + "%";
            room.upload.barCurrent.style.width =  progress + "%";

            let bytes = event.loaded - bytesPrev;
            accumulated += bytes;
            bytesPrev = event.loaded;

            let now     = new Date().getTime();
            let elapsed = (now - timeRate) / 1000.0;

            let loaded = formatByteCount(event.loaded);
            let total  = formatByteCount(event.total);
            room.upload.uploaded.textContent = loaded + " / " + total;

            if (elapsed > 1.0) {
                let rate = accumulated / elapsed;
                room.upload.transfer.textContent = formatByteCount(rate) + "/s";

                timeRate = now;
                accumulated = 0;
            }
        });

        upload.then(response => {
            if (response.checkError()) {
                return;
            }

            room.upload.placeholderRoot.classList.remove("hide");
            room.upload.progressRoot.classList.add("hide");

            let data = response.json;
            switch (data.category) {
                case "audio":
                case "video": {
                    this.entryArea.urlInput.value   = data.url;
                    this.entryArea.titleInput.value = data.name;
                } break;

                case "subs": {
                    this.entryArea.subtitleUrlInput.value  = data.url;
                    this.entryArea.subtitleNameInput.value = data.name;
                } break;
            }
        });
    }

    copyEntryToEntryArea(entry) {
        let area = this.entryArea;
        area.urlInput.value     = entry.url;
        area.titleInput.value   = entry.title;
        area.refererInput.value = entry.referer_url;

        if (entry.use_proxy) {
            area.proxyToggle.classList.add("active");
        } else {
            area.proxyToggle.classList.remove("active");
        }

        if (entry.subtitles && entry.subtitles.length > 0) {
            let sub = entry.subtitles[0];
            area.subtitleUrlInput.value  = sub.url;
            area.subtitleNameInput.value = sub.name;
        }
    }

    attachRoomTabEvents() {
        const room = this.roomContent;

        room.titleInput.onkeydown = event => {
            if (event.key === "Enter") {
                let title = room.titleInput.value.trim();
                room.titleInput.value = title;
                api.playerUpdateTitle(title);
            }
        };

        room.titleUpdateButton.onclick = _ => {
            let title = room.titleInput.value.trim();
            room.titleInput.value = title;
            api.playerUpdateTitle(title);
        };

        room.uploadSubButton.onclick = _ => {
            room.uploadSubInput.click();
        };

        room.uploadSubInput.onchange = async event => {
            let files = event.target.files;

            if (files.length === 0) {
                return;
            }

            let subtitle = await api.subtitleUpload(files[0], files[0].name);
            api.subtitleAttach(subtitle);
        };

        room.upload.placeholderRoot.onclick = _ => {
            room.upload.filepicker.click();
        };

        room.upload.placeholderRoot.ondragover = event => {
            event.preventDefault();
        };

        room.upload.placeholderRoot.ondrop = event => {
            event.preventDefault();

            let files = event.dataTransfer.files;
            if (!files) {
                return;
            }

            for (let i = 0; i < files.length; i++) {
                this.startMediaFileUpload(files[i]);
                break;
            }
        };

        room.upload.filepicker.onchange = event => {
            if (event.target.files.length === 0) {
                return;
            }

            this.startMediaFileUpload(event.target.files[0])
        };

        room.browse.videoButton.onclick     = _ => window.open("media/video/", "_blank").focus();
        room.browse.audioButton.onclick     = _ => window.open("media/audio/", "_blank").focus();
        room.browse.subtitlesButton.onclick = _ => window.open("media/subs/",  "_blank").focus();
        room.browse.imagesButton.onclick    = _ => window.open("media/image/", "_blank").focus();
        room.copyEntryButton.onclick        = _ => this.copyEntryToEntryArea(this.currentEntry);

        room.setShiftButton.onclick = _ => {
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

        room.subtitlesSelect.onchange = event => {
            let id  = Number(event.target.value)
            let sub = this.currentEntry.subtitles.find(sub => sub.id === id);

            room.subsEditInput.value = sub.name;
            this.roomSelectedSubId = sub.id;
        };

        room.subsUpdateButton.onclick = _ => {
            let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
            if (!subtitle) {
                return
            }

            let newName = room.subsEditInput.value.trim();
            api.subtitleUpdate(subtitle.id, newName);
        };

        room.subsDeleteButton.onclick = _ => {
            let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
            if (!subtitle) {
                return
            }

            api.subtitleDelete(subtitle.id);
        };

        room.openSettingsButton.onclick = _ => this.showSettingsMenu();
    }

    attachRightPanelEvents() {
        const tabs = this.rightPanel.tabs;

        tabs.room.onclick     = _ => this.selectRightPanelTab(TAB_ROOM);
        tabs.playlist.onclick = _ => this.selectRightPanelTab(TAB_PLAYLIST);
        tabs.chat.onclick     = _ => this.selectRightPanelTab(TAB_CHAT);
        tabs.history.onclick  = _ => this.selectRightPanelTab(TAB_HISTORY);

        this.attachRoomTabEvents();
        this.playlist.attachPlaylistEvents();
        this.chat.attachChatEvents();
        this.history.attachHistoryEvents();

        this.playlist.onSettingsClick   = _     => this.showSettingsMenu();
        this.history.onSettingsClick    = _     => this.showSettingsMenu();
        this.history.onContextEntryCopy = entry => this.copyEntryToEntryArea(entry);
    }

    async sendSetRequest() {
        let entry = await this.createNewRequestEntry();
        api.playerSet(entry).then(jsonResponse => {
            if (jsonResponse.checkError()) {
                return;
            }

            // Only reset if request was successful
            this.resetEntryAreaElements();
        });
    }

    attachEntryAreaEvents() {
        let area = this.entryArea;

        area.dropdownButton.onclick = _ => {
            area.root.classList.toggle("expand");
        };

        area.resetButton.onclick = _ => {
            this.resetEntryAreaElements();
        };

        area.urlInput.onkeydown = event => {
            if (event.key === "Enter") {
                this.sendSetRequest();
            }
        };

        area.setButton.onclick = _ =>  this.sendSetRequest();

        area.addPlaylistButton.onclick = async _ => {
            let entry = await this.createNewRequestEntry();
            if (entry.url) {
                api.playlistAdd(entry);
                this.resetEntryAreaElements();
            }
        };

        area.selectSubtitleButton.onclick = _ => {
            let input = fileInput(".srt,.vtt");

            input.onchange = event => {
                let files = event.target.files;

                if (files.length === 0) {
                    return;
                }

                console.log("File selected: ", files[0]);
                this.subtitleFile = files[0];
                area.subtitleNameInput.value = this.subtitleFile.name;
            }

            input.click();
        };

        area.youtubeSearchToggle.onclick   = _ => {
            const toggle = area.youtubeSearchToggle;
            toggle.classList.toggle("active");

            if (toggle.classList.contains("active")) {
                area.urlLabel.textContent = "YouTube video name";
            } else {
                area.urlLabel.textContent = "Entry URL";
            }
        };

        area.youtubePlaylistToggle.onclick  = _ => area.youtubePlaylistToggle.classList.toggle("active");
        area.addToTopToggle.onclick         = _ => area.addToTopToggle.classList.toggle("active");
        area.proxyToggle.onclick            = _ => area.proxyToggle.classList.toggle("active");
        area.subtitleReattachToggle.onclick = _ => area.subtitleReattachToggle.classList.toggle("active");
    }

    attachSettingsMenuEvents() {
        const menu = this.settingsMenu;
        menu.modal.onclick = _ => this.hideSettingsMenu();
        menu.root.onclick  = event => event.stopPropagation();

        menu.root.onkeydown = event => { 
            if (event.key === "Escape") {
                this.hideSettingsMenu();
            }
        };

        menu.closeButton.onclick = _ => this.hideSettingsMenu();

        menu.tokenCopyButton.onclick = _ => {
            navigator.clipboard.writeText(api.getToken());
        };

        menu.tokenSetButton.onclick = async _ => {
            let newToken = menu.tokenSetInput.value
            if (!newToken || newToken === "") {
                console.warn("WARN: Provided token is empty.");
                return;
            }

            let currToken = api.getToken();
            if (newToken === currToken) {
                console.warn("WARN: Provided token is the same as currently set token.");
                return;
            }

            let result = await api.userVerify(newToken);
            if (!result.ok) {
                console.warn("WARN: User with provided token does not exist.");
                return;
            }

            await api.userDelete(currToken);
            Storage.set("token", newToken);
            menu.tokenSetInput.value = "";
            window.location.reload();
        };

        menu.animatedAvatarsToggle.onclick = _ => {
            this.pageRoot.classList.toggle("disable_image_animations");
            let isToggled = menu.animatedAvatarsToggle.classList.toggle("active");
            Storage.setBool(USER_AVATAR_ANIMATIONS, isToggled);
        };

        menu.newMessageSoundToggle.onclick = _ => {
            let isToggled = menu.newMessageSoundToggle.classList.toggle("active");
            Storage.setBool(NEW_MESSAGE_SOUND, isToggled);
        };

        menu.theaterModeToggle.onclick = _ => {
            let isToggled = menu.theaterModeToggle.classList.toggle("active");
            Storage.setBool(ROOM_THEATER_MODE, isToggled);
            this.pageRoot.classList.toggle("theater_mode");
        };

        menu.themeSwitcherSelect.onchange = event => {
            let theme = event.target.value;
            this.selectedTheme.href = `css/themes/${theme}.css`
            Storage.set(SELECTED_THEME, theme)
        }

        menu.deleteYourAccountButton.onclick = _ => {
            if (menu.confirmAccountDelete.value === "I confirm") {
                api.userDelete(api.getToken());
            }
        };
    }

    attachHtmlEvents() {
        this.attachSettingsMenuEvents();
        this.attachEntryAreaEvents();
        this.usersArea.settingsButton.onclick = _ => this.showSettingsMenu();
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

        // Sorting dates in descending order
        this.allUsers.sort((a, b) => {
            let aDate = new Date(a.last_online);
            let bDate = new Date(b.last_online);
            return bDate.getTime() - aDate.getTime();
        });

        for (let i = 0; i < this.allUsers.length; i++) {
            let user = this.allUsers[i];
            let userbox = this.createUserBox(user);

            if (user.id === this.currentUserId) {
                selfBox = userbox;
            } else if (user.online) {
                onlineBoxes.push(userbox);
            } else {
                offlineBoxes.push(userbox);
            }

            if (user.online) {
                this.onlineCount += 1;
                userbox.root.classList.add("online");
            } 

            this.allUserBoxes.push(userbox);
        }

        userList.appendChild(selfBox.root);

        for (let i = 0; i < onlineBoxes.length; i++) {
            let box = onlineBoxes[i].root;
            userList.appendChild(box);
        }

        for (let i = 0; i < offlineBoxes.length; i++) {
            let box = offlineBoxes[i].root;
            userList.appendChild(box);
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
        console.info("INFO: Loaded users:", this.allUsers);

        this.clearUsersArea();
        this.updateUsersArea();
    }

    appendSelfUserContent(userbox) {
        let changeAvatarButton = button("user_box_change_avatar","Update your avatar");
        let uploadAvaterSvg    = svg("svg/main_icons.svg#upload");
        let shadowContainer    = div("user_box_shadow");
        let editNameButton     = button("user_box_edit_name_button", "Change your username");
        let editNameSvg        = svg("svg/main_icons.svg#edit2");

        //
        // Configuring parameters for html elements.
        //
        // changeAvatarButton.textContent = "E";
        userbox.root.classList.add("selfbox");

        //
        // Attaching events to html elements
        //
        changeAvatarButton.onclick = _ => {
            let input = fileInput(".png,.jpg,.jpeg,.gif,.webp")

            input.onchange = event => {
                let file = event.target.files[0];
                console.log("Picked file:", file);
                api.userUpdateAvatar(file) 
            }

            input.click();
        };
        userbox.nameInput.addEventListener("focusout", _ => {
            userbox.nameInput.readOnly = true;

            let user = this.allUsers.find(user => this.currentUserId === user.id);
            let newUsername = userbox.nameInput.value.trim();
            if (newUsername && newUsername !== user.username) {
                api.userUpdateName(newUsername);
            } else {
                userbox.nameInput.value = user.username;
            }
        });

        userbox.nameInput.addEventListener("keypress", event => {
            if (event.key === "Enter") {
                userbox.nameInput.readOnly = true;

                let user = this.allUsers.find(user => this.currentUserId === user.id);
                let newUsername = userbox.nameInput.value.trim()
                if (newUsername && newUsername !== user.username) {
                    api.userUpdateName(newUsername);
                } else {
                    userbox.nameInput.value = user.username;
                }
            }
        });

        // TODO(kihau): 
        //   Current edit behaviour is still pretty confusing. 
        //   Instead clicking edit name button should switch edit mode on and off (similar to the playlist entries)
        editNameButton.onclick = _ => {
            userbox.nameInput.readOnly = false;
            userbox.nameInput.focus();
        };

        //
        // Constructing html element structure
        //
        userbox.top.appendChild(shadowContainer);
        userbox.top.appendChild(changeAvatarButton); {
            changeAvatarButton.appendChild(uploadAvaterSvg);
        }
        userbox.bottom.appendChild(editNameButton); {
            editNameButton.appendChild(editNameSvg);
        }
    }

    createUserBox(user) {
        let root      = div("user_box");
        let top       = div("user_box_top");
        let avatar    = dynamicImg(user.avatar);
        let bottom    = div("user_box_bottom");
        let nameInput = document.createElement("input");

        let userbox = {
            root:      root,
            top:       top,
            bottom:    bottom,
            avatar:    avatar,
            nameInput: nameInput,
        }

        //
        // Configuring parameters for html elements.
        //
        nameInput.readOnly = true;
        nameInput.value = user.username;

        //
        // Constructing html element structure.
        //
        root.append(top); {
            top.append(avatar);
        }
        root.append(bottom); {
            bottom.append(nameInput);
        }

        if (user.id == this.currentUserId) {
            this.appendSelfUserContent(userbox, user);
        }

        return userbox;
    }

    async loadPlaylistData() {
        let entries = await api.playlistGet();
        console.info("INFO: Loaded playlist:", entries);

        this.playlist.clear();
        // TODO(kihau): Performance problem when number of entries is large. Needs to be fixed at some point.
        this.playlist.loadEntries(entries, this.allUsers);
    }

    async loadChatData() {
        let messages = await api.chatGet(MAX_CHAT_LOAD, 0);
        if (messages.checkError()) {
            return;
        }

        console.info("INFO: Loaded chat messages:", messages.json);
        this.chat.clear();
        this.chat.loadMessages(messages.json, this.allUsers);
    }

    async loadHistoryData() {
        let entries = await api.historyGet();
        console.info("INFO: Loaded history:", entries);

        this.history.clear();
        for (let i = 0; i < entries.length; i++) {
            const entry = entries[i];
            this.history.add(entry);
        }
    }

    updateRoomContent(entry) {
        this.roomContent.urlInput.value   = entry.url;
        this.roomContent.titleInput.value = entry.title;
        if (entry.use_proxy) {
            this.roomContent.usingProxyCheckbox.classList.add("active");
        } else {
            this.roomContent.usingProxyCheckbox.classList.remove("active");
        }

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

        let url = entry.url;
        if (!url) {
            this.setNothing();
            return;
        }

        if (entry.source_url) {
            url = entry.source_url;
        }

        this.player.setVideoTrack(url);

        if (entry.title) {
            this.player.setTitle(entry.title);
        }

        this.player.clearAllSubtitleTracks();
        if (entry.subtitles) {
            for (let i = 0; i < entry.subtitles.length; i++) {
                let sub = entry.subtitles[i];
                this.player.addSubtitle(sub.url, sub.name, sub.shift);
            }
        }

        this.player.setPoster(null)
        if (entry.thumbnail) {
            this.player.setPoster(entry.thumbnail);
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

        if (userId === 0) {
            console.info("INFO: Received resync event from SERVER at", timestamp, "with desync:", desync);
        } else {
            console.info("INFO: Received resync event from USER id", userId, "at", timestamp, "with desync:", desync);
        }

        if (Math.abs(desync) > MAX_DESYNC && !this.player.isLive()) {
            let diff = Math.abs(desync) - MAX_DESYNC
            console.warn("WARN: You are desynced! MAX_DESYNC(" + MAX_DESYNC + ") exceeded by:", diff, "Trying to resync now!");
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

            hide(this.connectionLostPopup);

            await this.loadUsersData();
            await this.loadPlayerData();
            await this.loadPlaylistData();
            await this.loadChatData();
            await this.loadHistoryData();
            api.uptime().then(uptime   => this.settingsMenu.websiteUptime.textContent  = uptime);
            api.version().then(version => this.settingsMenu.websiteVersion.textContent = version);
        };

        events.onerror = _ => {
            events.close()
            console.error("ERROR: Connection to the server was lost. Attempting to reconnect in", RECONNECT_AFTER, "ms");
            this.handleDisconnect();
        };

        this.subscribeToServerEvents(events);

        window.addEventListener("beforeunload", _ => {
            events.close();
        });
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
                console.warn("WARN: Subtitle delete event failed, event data is null.")
                return;
            }

            let subId = JSON.parse(event.data);
            console.info("INFO: Received subtitle delete event for subtitle with ID:", subId);

            let subs = this.currentEntry.subtitles;
            if (!subs) {
                console.warn("WARN: Subtitle delete event failed, currentEntry subtitles is null.")
                return;
            }

            let index = subs.findIndex(sub => sub.id === subId);
            if (index === -1) {
                console.warn("WARN: Subtitle delete event failed, subtitle index is -1.")
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
                console.warn("WARN: Subtitle update event failed, event data is null.")
                return;
            }

            console.info("INFO: Received subtitle update event with:", data);

            let subs = this.currentEntry.subtitles;
            if (!subs) {
                console.warn("WARN: Subtitle update event failed, currentEntry subtitles is null.")
                return;
            }

            let index = subs.findIndex(sub => sub.id === data.id);
            if (index === -1) {
                console.warn("WARN: Subtitle update event failed, subtitle index is -1.")
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

            let userbox = this.createUserBox(user);
            this.allUserBoxes.push(userbox);
            this.usersArea.userList.appendChild(userbox.root);

            this.usersArea.onlineCount.textContent = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userdelete", event => {
            let target = JSON.parse(event.data)
            let index = this.allUsers.findIndex(user => user.id === target.id);

            if (this.currentUserId == target.id) {
                events.close();
                this.markAllUsersOffline();
                this.clearUsersArea();
                this.updateUsersArea();
            }

            let user = this.allUsers.splice(index, 1)[0];
            let userBox = this.allUserBoxes.splice(index, 1)[0];

            console.info("INFO: Removing user:", user, "with its user box", userBox);
            this.usersArea.userList.removeChild(userBox.root);

            let online = 0;
            for (let i = 0; i < this.allUsers.length; i++) {
                if (this.allUsers[i].online) online += 1;
                console.warn(this.allUsers[i], online)
            }

            this.onlineCount = online;

            this.usersArea.onlineCount.textContent  = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        // All user-related update events can be multiplexed into one "user-update" event to simplify logic
        // The server will always serve the up-to-date snapshot of User which should never exceed 1 kB in practice
        events.addEventListener("userconnected", event => {
            let userId = JSON.parse(event.data);
            console.info("INFO: User connected, ID: ", userId)

            let userBoxes = this.usersArea.userList;
            let onlineBoxes = userBoxes.getElementsByClassName("online");
            let lastOnlineBox = onlineBoxes[onlineBoxes.length - 1];

            let index = this.allUsers.findIndex(user => user.id === userId);
            if (index === -1) {
                console.warn("WARN: Failed to find users with user ID =", userId);
                return;
            }

            this.allUsers[index].online = true;
            this.allUserBoxes[index].root.classList.add("online");

            let connectedNow = this.allUserBoxes[index].root;
            if (lastOnlineBox) {
                userBoxes.insertBefore(connectedNow, lastOnlineBox.nextSibling);
            } else {
                userBoxes.appendChild(connectedNow);
            }

            this.onlineCount += 1;

            this.usersArea.onlineCount.textContent  = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userdisconnected", event => {
            let userId = JSON.parse(event.data);
            console.info("INFO: User disconnected, ID: ", userId)

            let userBoxes = this.usersArea.userList;
            let offlineBoxes = userBoxes.getElementsByClassName("user_box.online");

            // let firstOfflineBox = offlineBoxes[offlineBoxes.length - 1].nextElementSibling;
            let lastOnlineBox = offlineBoxes[offlineBoxes.length - 1];

            let index = this.allUsers.findIndex(user => user.id === userId);
            if (index === -1) {
                console.warn("WARN: Failed to find users with user ID =", userId);
                return
            }

            this.allUsers[index].online = false;
            this.allUserBoxes[index].root.classList.remove("online");

            let disconnectedNow = this.allUserBoxes[index].root;
            if (lastOnlineBox && lastOnlineBox.nextElementSibling) {
                userBoxes.insertBefore(disconnectedNow, lastOnlineBox.nextElementSibling);
            } else {
                userBoxes.appendChild(disconnectedNow);
            }

            this.onlineCount -= 1;

            this.usersArea.onlineCount.textContent  = this.onlineCount;
            this.usersArea.offlineCount.textContent = this.allUsers.length - this.onlineCount;
        });

        events.addEventListener("userupdate", event => {
            let user = JSON.parse(event.data);
            console.info("INFO: Update user name event for: ", user)

            let index = this.allUsers.findIndex(x => x.id == user.id);
            if (index === -1) {
                console.warn("WARN: Failed to find users with user ID =", user.id);
                return;
            }

            this.allUsers[index] = user;

            let userbox = this.allUserBoxes[index]; 

            let input = userbox.nameInput;
            input.value = user.username;
           
            let newAvatar = dynamicImg(user.avatar);
            userbox.avatar.replaceWith(newAvatar);
            userbox.avatar = newAvatar;

            this.playlist.handleUserUpdate(user);
        });

        events.addEventListener("playerset", event => {
            let entry = JSON.parse(event.data);
            console.info("INFO: Received player set event: ", entry);
            this.setEntryEvent(entry);
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
            this.currentEntry.title = title;
            this.roomContent.titleInput.value = title;
        });

        events.addEventListener("playerwaiting", event => {
            let message = JSON.parse(event.data);
            this.player.setToast(message);
            this.player.setPoster("/watch/img/please_stand_by.webp");
            this.player.setBuffering(true);
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
                    if (userId !== SERVER_ID) {
                        this.player.setToast(username + " clicked play.");
                        this.roomContent.lastActionText.textContent = username + " clicked play.";
                    }

                    this.resyncPlayer(timestamp, userId);
                    this.player.play();
                } break;

                case "pause": {
                    if (userId !== SERVER_ID) {
                        this.player.setToast(username + " clicked pause.");
                        this.roomContent.lastActionText.textContent = username + " clicked pause.";
                    }

                    this.resyncPlayer(timestamp, userId);
                    this.player.pause();
                } break;

                case "seek": {
                    if (userId !== SERVER_ID) {
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

            let messageSoundEnabled = this.settingsMenu.newMessageSoundToggle.classList.contains("active");
            if (messageSoundEnabled && (this.selected_tab !== this.rightPanel.tabs.chat || this.player.isFullscreen())) {
                this.newMessageAudio.play();
            }

            this.chat.addMessage(data, this.allUsers);
        });

        events.addEventListener("historyclear", _ => {
            console.info("INFO: Received history clear event");
            this.history.clear();
        });

        events.addEventListener("historyadd", event => {
            let entry = JSON.parse(event.data);
            console.info("INFO: Received history addevent: ", entry);
            this.history.add(entry);
        });

        events.addEventListener("historyremove", event => {
            let entryId = JSON.parse(event.data);
            console.info("INFO: Received history addremove: ", entryId);
            this.history.remove(entryId);
        });
    }

    handleDisconnect() {
        this.markAllUsersOffline();
        this.clearUsersArea();
        this.updateUsersArea();
        this.player.setToast("Connection to the server was lost...");
        show(this.connectionLostPopup);
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
