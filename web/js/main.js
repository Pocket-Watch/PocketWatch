import { Options, Player, Timeout } from "./custom_player.js";
import { Playlist } from "./playlist.js";
import { History } from "./history.js";
import { Chat } from "./chat.js";
import { sha256 } from "./auth.js";
import * as api from "./api.js";
import { Storage, button, div, formatTime, formatByteCount, getById, dynamicImg, svg, show, hide, fileInput, isLocalUrl, input } from "./util.js";

const SERVER_ID = 0;

const VERSION                = "version";
const SELECTED_THEME         = "selected_theme";
const USER_AVATAR_ANIMATIONS = "user_avatar_animations";
const NEW_MESSAGE_SOUND      = "new_message_sound";
const ROOM_THEATER_MODE      = "room_theater_mode";
const LOW_BANDWIDTH_MODE     = "low_bandwidth_mode";
const LAST_SELECTED_TAB      = "last_selected_tab";
const LAST_SELECTED_SUBTITLE = "last_selected_subtitle";
const HLS_DEBUG              = "hls_debug";

const TAB_DEFAULT  = 0;
const TAB_ROOM     = 1;
const TAB_PLAYLIST = 2;
const TAB_CHAT     = 3;
const TAB_HISTORY  = 4;

const RECONNECT_AFTER = 1500;
const MAX_CHAT_LOAD = 100;

class Room {
    constructor() {
        let video0 = getById("video0");

        let options                = new Options();
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

        this.pageIcon      = getById("page_icon");
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
            hlsDebugToggle:          getById("hls_debug_toggle"),
            lowBandwidthModeToggle:  getById("low_bandwidth_mode_toggle"),
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
            uploadSubInput:  getById("room_subtitle_upload_input"),
            subsEditInput:   getById("room_subtitle_name_input"),

            titleUpdateButton:  getById("room_title_update_button"),
            urlCopyButton:      getById("room_entry_url_copy_button"),
            uploadSubButton:    getById("room_subtitle_upload_button"),
            copyEntryButton:    getById("room_copy_entry_button"),
            setShiftButton:     getById("room_set_shift_button"),
            subsUpdateButton:   getById("room_subtitle_update_button"),
            subsDeleteButton:   getById("room_subtitle_delete_button"),
            openSettingsButton: getById("room_open_settings_button"),

            usingProxyCheckbox: getById("room_entry_proxy_enabled"),
            uploadFileProgress: getById("room_upload_file_progress"),
            createdByUsername:  getById("room_created_by_username"),
            lastActionText:     getById("room_last_action_text"),
            subtitlesSelect:    getById("room_subtitle_select"),
            videoResolution:    getById("room_video_resolution"),
            createdAtDate:      getById("room_created_at_date"),

            upload: {
                placeholderRoot: getById("room_upload_media_placeholder"),
                progressRoot:    getById("room_upload_media_progress"),
                filePicker:      getById("room_upload_media_filepicker"),
                text:            getById("room_upload_media_progress_text"),
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

        this.selected_tab = TAB_DEFAULT;

        this.roomSelectedSubId = -1;

        // Self user id. Server User structure.
        this.currentUserId = -1;

        // List of all users in current room.
        this.allUsers = [];

        // List of all html user elements displayed inside of users_list element.
        this.allUserBoxes = [];

        // Number of user online.
        this.onlineCount = 0;

        // Subtitle file to be attached to the entry.
        this.subtitleFile = null;

        // ID of the currently set entry.
        this.currentEntryId = 0;
        this.currentEntry = {};

        // Player state on website load.
        this.stateOnLoad = null;

        // Timeouts
        this.fileUploadReset  = new Timeout(_ => {
            this.roomContent.upload.placeholderRoot.classList.remove("hide");
            this.roomContent.upload.progressRoot.classList.add("hide");
        }, 2000);
    }

    showSettingsMenu(_settingsTab) {
        show(this.settingsMenu.modal);
        this.settingsMenu.root.focus();
    }

    hideSettingsMenu() {
        hide(this.settingsMenu.modal);
    }

    configureHlsRequests(xhr, url) {
        if (isLocalUrl(url)) {
            xhr.setRequestHeader("Authorization", api.getToken());
        }
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

        let enabled = Storage.getBool(HLS_DEBUG);
        if (enabled) {
            options.hlsConfig.debug = enabled;
        }

        let disabled = Storage.getBool(Options.VIDEO_DISABLED);
        if (disabled) {
            options.disableVideo = disabled;
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

        if (Storage.getBool(ROOM_THEATER_MODE)) {
            this.pageRoot.classList.add("theater_mode");
            this.settingsMenu.theaterModeToggle.classList.add("active");
        } else {
            this.pageRoot.classList.remove("theater_mode");
            this.settingsMenu.theaterModeToggle.classList.remove("active");
        }

        let enabled = Storage.getBool(HLS_DEBUG);
        if (enabled) {
            this.settingsMenu.hlsDebugToggle.classList.add("active");
        }

        if (Storage.getBool(LOW_BANDWIDTH_MODE)) {
            this.settingsMenu.lowBandwidthModeToggle.classList.add("active");
            // Replace images with placeholders?
        } else {
            this.settingsMenu.lowBandwidthModeToggle.classList.remove("active");
            // Now load the images or demand a page reload?
        }

        let theme = Storage.get(SELECTED_THEME);
        if (theme) {
            this.selectedTheme.href = `css/themes/${theme}.css`;
            this.settingsMenu.themeSwitcherSelect.value = theme;
        }

        // Player settings
        let volume = Storage.get("volume");
        if (volume !== null) {
            this.player.setVolume(volume);
        }

        let muted = Storage.getBool("muted");
        if (muted !== null && muted && volume > 0) {
            this.player.toggleMute();
        }

        let lastSub = Storage.get(LAST_SELECTED_SUBTITLE);
        if (lastSub !== null) {
            this.player.switchSubtitleTrackByUrl(lastSub);
        }

        let subsEnabled = Storage.getBool(Options.SUBTITLES_ENABLED);
        if (subsEnabled) {
            this.player.enableSubtitles();
        }

        let size = Storage.get(Options.SUBTITLE_FONT_SIZE);
        if (size !== null) {
            this.player.setSubtitleFontSize(size);
        }

        let position = Storage.getNum(Options.SUBTITLE_VERTICAL_POSITION);
        if (position !== null) {
            this.player.setSubtitleVerticalPosition(position);
        }

        let fgColor = Storage.get(Options.SUBTITLE_FOREGROUND_COLOR);
        let fgOpacity = Storage.get(Options.SUBTITLE_FOREGROUND_OPACITY);
        if (fgColor !== null && fgOpacity !== null) {
            this.player.setSubtitleForegroundColor(fgColor, fgOpacity);
        }

        let bgColor = Storage.get(Options.SUBTITLE_BACKGROUND_COLOR);
        let bgOpacity = Storage.get(Options.SUBTITLE_BACKGROUND_OPACITY);
        if (bgColor !== null && bgOpacity !== null) {
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
                api.wsPlayerPlay(0);
            } else {
                api.wsPlayerPlay(this.player.getCurrentTime());
            }
        });

        this.player.onControlsPause(_ => {
            api.wsPlayerPause(this.player.getCurrentTime());
        });

        this.player.onControlsSeeked(timestamp => {
            api.wsPlayerSeek(timestamp);
        });

        this.player.onControlsSeeking(timestamp => {
            console.log("User seeking to", timestamp);
        });

        this.player.onControlsNext(_ => {
            api.wsPlayerNext(this.currentEntryId);
        });

        // Maybe browsers optimize calls to localStorage and don't write to disk 30 times a second?
        this.player.onControlsVolumeSet(volume => {
            Storage.set("volume", volume);
        });

        this.player.onControlsMute(muted => {
            Storage.setBool("muted", muted);
        });

        this.player.onSettingsChange((key, value) => {
            switch (key) {
                case Options.SHOW_CONTROLS_ON_PAUSE:
                case Options.ALWAYS_SHOW_CONTROLS:
                case Options.SUBTITLES_ENABLED:
                case Options.VIDEO_DISABLED:
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
                api.wsPlayerNext(this.currentEntryId);
            } else {
                console.info("INFO: Playback ended! Informing the server");
                let endTime = this.player.getDuration();
                if (isNaN(endTime)) {
                    endTime = 0;
                }

                api.wsPlayerPause(endTime);
            }
        });

        this.player.onSubtitleSelect(subtitle => {
            Storage.set(LAST_SELECTED_SUBTITLE, subtitle.url);
        });

        this.player.onMetadataLoad(_ => {
            this.roomContent.videoResolution.textContent = this.player.getResolution();
        });

        this.player.onPlaybackError((exception, error) => {
            if (exception.name === "NotAllowedError") {
                this.player.setToast("Auto-play is disabled by your browser!\nClick anywhere on the player to start the playback.");
                return;
            }

            if (exception.name === "AbortError") {
                this.player.setToast("AbortError: Likely the video is slowly loading. Pausing playback!");
                api.wsPlayerPause(this.player.getCurrentTime());
                return;
            }

            if (!error) {
                this.player.setToast("UNKNOWN ERROR, press F12 to see what happened!");
                console.error(exception.name + ":", exception.message);
                api.wsPlayerPause(this.player.getCurrentTime());
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
                    api.wsPlayerPause(this.player.getCurrentTime());
                    return;
                }

                if (errMsg.startsWith("404")) {
                    this.player.setToast("Resource not found [404]!");
                } else {
                    this.player.setToast("Unsupported src: '" + this.player.getCurrentUrl() + "' " + error.message);
                }

                api.wsPlayerPause(this.player.getCurrentTime());
            }
        });

        // NOTE(kihau): This hack fixes HLS issues with seeking on website load and autoplaying.
        this.player.onDataLoad(_ => {
            if (this.stateOnLoad) {
                this.player.seek(this.stateOnLoad.timestamp);
                if (this.stateOnLoad.playing) {
                    this.player.play();
                }

                this.stateOnLoad = null;
            } else if (this.playlist.autoplayEnabled) {
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

        let subName = area.subtitleNameInput.value.trim();
        let subUrl  = area.subtitleUrlInput.value.trim();
        let referer = area.refererInput.value.trim();

        let subtitles = [];

        let sub;
        if (this.subtitleFile) {
            sub = await api.subtitleUpload(this.subtitleFile, subName);
        } else if (subUrl) {
            let response = await api.subtitleDownload(subUrl, subName, referer);
            sub = response.json;
        }

        if (sub) {
            subtitles.push(sub);
        }

        if (area.subtitleReattachToggle.classList.contains("active")) {
            subtitles = subtitles.concat(this.currentEntry.subtitles);
        }

        let countString = area.ytCountInput.value.trim();
        let count = Number(countString);
        if (!count || count <= 0) {
            count = 20;
        }

        let skipCountString = area.ytSkipCountInput.value.trim();
        let skipCount = Number(skipCountString);
        if (!skipCount || skipCount <= 0) {
            skipCount = 0;
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

    getRightPanelTabHtml(tab_type) {
        let tab;
        let content;
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

        let htmlContent = {
            tab:     tab,
            content: content,
        };

        return htmlContent;
    }

    selectRightPanelTab(tab_type) {
        const selected = this.getRightPanelTabHtml(this.selected_tab);

        selected.tab.classList.remove("selected");
        selected.content.classList.remove("selected");

        const { tab, content } = this.getRightPanelTabHtml(tab_type);

        tab.classList.add("selected");
        content.classList.add("selected");

        if (this.selected_tab === tab_type) {
            content.scrollIntoView({ behavior: "smooth", block: "end" });
        }

        if (tab_type === TAB_CHAT) {
            hide(this.chatNewMessage);
        }

        this.selected_tab = tab_type;
        Storage.set(LAST_SELECTED_TAB, tab_type);
    }

    startMediaFileUpload(file) {
        this.fileUploadReset.cancel();
        const room = this.roomContent;

        let timeRate = new Date().getTime();
        // Byte size accumulated after each second.
        let accumulated = 0;

        let bytesPrev = 0;

        room.upload.percent.textContent    = "0%";
        room.upload.barCurrent.style.width = "0%";
        room.upload.uploaded.textContent   = "0 B / 0 B";
        room.upload.transfer.textContent   = "0 B/s";

        room.upload.text.textContent = "Uploading file";
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
            this.fileUploadReset.schedule();
            if (response.checkError()) {
                room.upload.text.textContent = "Error: " + response.error.message;
                return;
            }

            room.upload.text.textContent = "Upload finished!";

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
                api.wsPlayerUpdateTitle(title);
            }
        };

        room.titleUpdateButton.onclick = _ => {
            let title = room.titleInput.value.trim();
            room.titleInput.value = title;
            api.wsPlayerUpdateTitle(title);
        };

        room.urlCopyButton.onclick = _ => {
            navigator.clipboard.writeText(this.currentEntry.url);
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
            await api.subtitleAttach(subtitle);
        };

        room.upload.placeholderRoot.onclick = _ => {
            room.upload.filePicker.click();
        };

        room.upload.placeholderRoot.ondragover = event => {
            event.preventDefault();
        };

        room.upload.placeholderRoot.ondrop = event => {
            event.preventDefault();

            let files = event.dataTransfer.files;
            if (!files || files.length === 0) {
                return;
            }

            this.startMediaFileUpload(files[0]);
        };

        room.upload.filePicker.onchange = event => {
            if (event.target.files.length === 0) {
                return;
            }

            this.startMediaFileUpload(event.target.files[0]);
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
            let id  = Number(event.target.value);
            let sub = this.currentEntry.subtitles.find(sub => sub.id === id);

            room.subsEditInput.value = sub.name;
            this.roomSelectedSubId = sub.id;
        };

        room.subsEditInput.onkeydown = event => {
            if (event.key === "Enter") {
                let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
                if (!subtitle) {
                    return;
                }

                let newName = room.subsEditInput.value.trim();
                api.subtitleUpdate(subtitle.id, newName);
            }
        };

        room.subsUpdateButton.onclick = _ => {
            let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
            if (!subtitle) {
                return;
            }

            let newName = room.subsEditInput.value.trim();
            api.subtitleUpdate(subtitle.id, newName);
        };

        room.subsDeleteButton.onclick = _ => {
            let subtitle = this.currentEntry.subtitles.find(sub => sub.id === this.roomSelectedSubId);
            if (!subtitle) {
                return;
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
                api.wsPlaylistAdd(entry);
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
            };

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

    async setNewToken() {
        let newToken = this.settingsMenu.tokenSetInput.value;
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
        this.settingsMenu.tokenSetInput.value = "";
        window.location.reload();
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

        menu.tokenCopyButton.onclick = async _ => {
            if (navigator.clipboard) {
                await navigator.clipboard.writeText(api.getToken());
            } else {
                menu.tokenSetInput.value = api.getToken();
            }
        };

        menu.tokenSetInput.onkeydown = async event => {
            if (event.key === "Enter") {
                await this.setNewToken();
            }
        };

        menu.tokenSetButton.onclick = async _ => {
            await this.setNewToken();
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

        menu.hlsDebugToggle.onclick = _ => {
            let isToggled = menu.hlsDebugToggle.classList.toggle("active");
            Storage.setBool(HLS_DEBUG, isToggled);
        };

        menu.lowBandwidthModeToggle.onclick = _ => {
            let isToggled = menu.lowBandwidthModeToggle.classList.toggle("active");
            Storage.setBool(LOW_BANDWIDTH_MODE, isToggled);
            // Do something or page reload?
        };

        menu.themeSwitcherSelect.onchange = event => {
            let theme = event.target.value;
            this.selectedTheme.href = `css/themes/${theme}.css`;
            Storage.set(SELECTED_THEME, theme);
        };

        menu.deleteYourAccountButton.onclick = _ => {
            if (menu.confirmAccountDelete.value === "I confirm") {
                api.userDelete(api.getToken());
            }
        };
    }

    attachHtmlEvents() {
        document.addEventListener("visibilitychange", _ => {
            if (document.visibilityState === "visible") {
                this.pageIcon.href = "img/favicon.ico";
            }
        });

        this.pageRoot.addEventListener("keydown", event => {
            if (event.key === "f" && event.target === this.pageRoot) {
                this.player.toggleFullscreen();
            }

            let ctrl = event.getModifierState("Alt");
            let alt  = event.getModifierState("Control");

            if (!ctrl || !alt) {
                return
            }

            switch (event.key) {
                case "r": {
                    this.selectRightPanelTab(TAB_ROOM);
                } break;

                case "p": {
                    this.selectRightPanelTab(TAB_PLAYLIST);
                } break;

                case "c": {
                    this.selectRightPanelTab(TAB_CHAT);
                    this.chat.chatInput.focus();
                } break;

                case "h": {
                    this.selectRightPanelTab(TAB_HISTORY);
                } break;

                case "d": {
                    this.entryArea.root.classList.toggle("expand");
                } break;
            }
        });

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

    async authenticateAccount() {
        let token = Storage.get("token");
        api.setToken(token);

        let result = await api.userVerify(token);
        if (result.ok) {
            this.currentUserId      = result.json;
            this.chat.currentUserId = result.json;
            return true;
        }

        // Server returned code 400 (Bad Request). That means, user token was not found.
        if (result.status !== 400) {
            return false;
        }

        // Temporary workaround for lack of persistent server-side account storage.
        token = await api.userCreate();
        if (token === null) {
            return false;
        }

        api.setToken(token);
        Storage.set("token", token);

        result = await api.userVerify(token);
        if (!result.ok) {
            return false;
        }

        this.currentUserId      = result.json;
        this.chat.currentUserId = result.json;
        return true;
    }

    async loadPlayerData() {
        let state = await api.playerGet();
        if (!state) {
            return;
        }

        this.playlist.setAutoplay(state.player.autoplay);
        this.playlist.setLooping(state.player.looping);

        this.setEntryEvent(state.entry);
        if (state.entry.id !== 0) {
            this.stateOnLoad = state.player;
        }
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

        if (selfBox) {
            userList.appendChild(selfBox.root);
        }

        for (let i = 0; i < onlineBoxes.length; i++) {
            let box = onlineBoxes[i].root;
            userList.appendChild(box);
        }

        for (let i = 0; i < offlineBoxes.length; i++) {
            let box = offlineBoxes[i].root;
            userList.appendChild(box);
        }

        this.usersArea.onlineCount.textContent  = this.onlineCount;
        this.usersArea.offlineCount.textContent = String(this.allUsers.length - this.onlineCount);
    }

    markAllUsersOffline() {
        for (let i = 0; i < this.allUsers.length; i++) {
            this.allUsers[i].online = false;
        }

        for (let i = 0; i < this.allUserBoxes.length; i++) {
            this.allUserBoxes[i].root.classList.remove("online");
        }

        this.onlineCount = 0;

        this.usersArea.onlineCount.textContent  = this.onlineCount;
        this.usersArea.offlineCount.textContent = String(this.allUsers.length - this.onlineCount);
    }

    async loadUsersData() {
        this.allUsers = await api.userGetAll();
        if (!this.allUsers) {
            this.allUsers = [];
        }

        console.info("INFO: Loaded users:", this.allUsers);

        this.clearUsersArea();
        this.updateUsersArea();
    }

    appendSelfUserContent(userbox) {
        let changeAvatarButton = button("user_box_change_avatar","Update your avatar");
        let uploadAvatarSvg    = svg("svg/main_icons.svg#upload");
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
            let input = fileInput(".png,.jpg,.jpeg,.gif,.webp");

            input.onchange = async event => {
                let file = event.target.files[0];
                console.log("Picked file:", file);
                await api.userUpdateAvatar(file);
            };

            input.click();
        };
        userbox.nameInput.addEventListener("focusout", async _ => {
            userbox.nameInput.readOnly = true;

            let user = this.allUsers.find(user => this.currentUserId === user.id);
            let newUsername = userbox.nameInput.value.trim();
            if (newUsername && newUsername !== user.username) {
                await api.userUpdateName(newUsername);
            } else {
                userbox.nameInput.value = user.username;
            }
        });

        userbox.nameInput.addEventListener("keypress", async event => {
            if (event.key === "Enter") {
                userbox.nameInput.readOnly = true;

                let user = this.allUsers.find(user => this.currentUserId === user.id);
                let newUsername = userbox.nameInput.value.trim();
                if (newUsername && newUsername !== user.username) {
                    await api.userUpdateName(newUsername);
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
            changeAvatarButton.appendChild(uploadAvatarSvg);
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
        let nameInput = input(null, user.username);

        let userbox = {
            root:      root,
            top:       top,
            bottom:    bottom,
            avatar:    avatar,
            nameInput: nameInput,
        };

        //
        // Configuring parameters for html elements.
        //
        nameInput.readOnly = true;

        //
        // Constructing html element structure.
        //
        root.append(top); {
            top.append(avatar);
        }
        root.append(bottom); {
            bottom.append(nameInput);
        }

        if (user.id === this.currentUserId) {
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
        // TODO(kihau): Nicer way to display the creation date.
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

        if (entry.proxy_url) {
            url = entry.proxy_url;
        } else if (entry.source_url) {
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

        this.player.setPoster(null);
        if (entry.thumbnail) {
            this.player.setPoster(entry.thumbnail);
        }
    }

    setNothing() {
        Storage.remove(LAST_SELECTED_SUBTITLE);

        this.player.discardPlayback();
        this.player.setTitle(null);
        this.player.setPoster("");
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
            let diff = Math.abs(desync) - MAX_DESYNC;
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
        let wsPrefix = window.location.protocol.startsWith("https:") ? "wss://" : "ws://";
        let ws = new WebSocket(wsPrefix + window.location.host + "/api/events?token=" + api.getToken());
        ws.onopen = async _ => {
            console.info("INFO: Connection to events opened.");

            hide(this.connectionLostPopup);
            api.setWebSocket(ws);

            await this.loadUsersData();
            this.loadPlayerData();
            this.loadPlaylistData();
            this.loadChatData();
            this.loadHistoryData();
            api.uptime().then(uptime   => this.settingsMenu.websiteUptime.textContent  = uptime);
            api.version().then(version => this.settingsMenu.websiteVersion.textContent = version);
        };

        ws.onclose = _ => {
            console.error("ERROR: Connection to the server was lost. Attempting to reconnect in", RECONNECT_AFTER, "ms");
            api.setWebSocket(null);
            this.handleDisconnect();
        };

        ws.onmessage = event => {
            let wsEvent = JSON.parse(event.data);
            this.handleServerEvent(wsEvent.type, wsEvent.data);
        }
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

    handleServerEvent(wsType, wsData) {
        switch (wsType) {
            case "welcome": {
                let lastVersion = Storage.get(VERSION);
                let version = wsData.version;

                if (!lastVersion) {
                    Storage.set(VERSION, version);
                    return;
                }
                if (lastVersion !== version) {
                    Storage.set(VERSION, version);
                    console.log("INFO: Reloading because the server version changed:", lastVersion, "->", version);
                    window.location.reload();
                }
            } break;

            case "ping": {
                // TODO(kihau): Respond with pong.
            } break;

            case "usercreate": {
                let user = wsData;
                this.allUsers.push(user);
                console.info("INFO: New user has been created: ", user);

                let userbox = this.createUserBox(user);
                this.allUserBoxes.push(userbox);
                this.usersArea.userList.appendChild(userbox.root);

                this.usersArea.onlineCount.textContent = this.onlineCount;
                this.usersArea.offlineCount.textContent = String(this.allUsers.length - this.onlineCount);

            } break;

            case "userdelete": {
                let target = wsData;
                let index = this.allUsers.findIndex(user => user.id === target.id);

                if (this.currentUserId === target.id) {
                    api.closeWebSocket();
                    this.markAllUsersOffline();
                }

                let user = this.allUsers.splice(index, 1)[0];
                let userBox = this.allUserBoxes.splice(index, 1)[0];

                console.info("INFO: Removing user:", user, "with its user box", userBox);
                this.usersArea.userList.removeChild(userBox.root);

                let online = 0;
                for (let i = 0; i < this.allUsers.length; i++) {
                    if (this.allUsers[i].online) online += 1;
                }

                this.onlineCount = online;

                this.usersArea.onlineCount.textContent  = this.onlineCount;
                this.usersArea.offlineCount.textContent = String(this.allUsers.length - this.onlineCount);
            } break;

            // All user-related update events can be multiplexed into one "user-update" event to simplify logic
            // The server will always serve the up-to-date snapshot of User which should never exceed 1 kB in practice
            case "userconnected": {
                let userId = wsData;
                console.info("INFO: User connected, ID: ", userId);

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
                this.usersArea.offlineCount.textContent = String(this.allUsers.length - this.onlineCount);
            } break;

            case "userdisconnected": {
                let userId = wsData;
                console.info("INFO: User disconnected, ID: ", userId);

                let userBoxes = this.usersArea.userList;
                let onlineBoxes = userBoxes.getElementsByClassName("user_box online");
                let lastOnlineBox = onlineBoxes[onlineBoxes.length - 1];

                let index = this.allUsers.findIndex(user => user.id === userId);
                if (index === -1) {
                    console.warn("WARN: Failed to find users with user ID =", userId);
                    return;
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
                this.usersArea.offlineCount.textContent = String(this.allUsers.length - this.onlineCount);
            } break;

            case "userupdate": {
                let user = wsData;
                console.info("INFO: Update user name event for: ", user);

                let index = this.allUsers.findIndex(x => x.id === user.id);
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
            } break;

            case "playerset": {
                let entry = wsData;
                console.info("INFO: Received player set event: ", entry);
                this.setEntryEvent(entry);
            } break;

            case "playerlooping": {
                let looping = wsData;
                this.playlist.setLooping(looping)
            } break;

            case "playerautoplay": {
                let autoplay = wsData;
                this.playlist.setAutoplay(autoplay);
            } break;

            case "playerupdatetitle": {
                let title = wsData;
                this.player.setTitle(title);
                this.currentEntry.title = title;
                this.roomContent.titleInput.value = title;
            } break;

            case "playerwaiting": {
                let message = wsData;
                console.info("INFO: Received player waiting event: ", message);

                this.player.setToast(message);
                this.player.setPoster("/watch/img/please_stand_by.webp");
                this.player.setBuffering(true);
            } break;

            case "playererror": {
                let message = wsData;
                console.info("INFO: Received player error event: ", message);

                this.player.setToast(message);
                this.player.setPoster("");
                this.player.setBuffering(false);
            } break;

            case "sync": {
                let data = wsData;
                if (!data) {
                    console.error("ERROR: Failed to parse event data");
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
            } break;

            case "playlist": {
                let response = wsData;
                console.info("INFO: Received playlist event for:", response.action, "with:", response.data);
                this.playlist.handleServerEvent(response.action, response.data, this.allUsers);
            } break;

            case "messagecreate": {
                let message = wsData;
                console.info("INFO: New message received from server");

                if (this.selected_tab !== TAB_CHAT) {
                    show(this.chatNewMessage);
                }

                if (this.shouldPlayNotificationSound(message.authorId)) {
                    this.newMessageAudio.play();
                }

                if (document.visibilityState === "hidden") {
                    this.pageIcon.href = "img/favicon_unread.ico";
                }

                this.chat.addMessage(message, this.allUsers);
            } break;

            case "messageedit": {
                let message = wsData;
                console.info("INFO: A message has been edited:", message);
                this.chat.edit(message.message_id, message.content);
            } break;

            case "messagedelete": {
                let messageId = wsData;
                console.info("INFO: Deleting message with ID =", messageId);
                this.chat.delete(messageId, this.allUsers);
            } break;

            case "historyclear": {
                console.info("INFO: Received history clear event");
                this.history.clear();
            } break;

            case "historyadd": {
                let entry = wsData;
                console.info("INFO: Received history add event: ", entry);
                this.history.add(entry);
            } break;

            case "historydelete": {
                let entryId = wsData;
                console.info("INFO: Received history delete event: ", entryId);
                this.history.delete(entryId);
            } break;

            case "subtitledelete": {
                let subId = wsData;
                console.info("INFO: Received subtitle delete event for subtitle with ID:", subId);

                let subs = this.currentEntry.subtitles;
                if (!subs) {
                    console.warn("WARN: Subtitle delete event failed, currentEntry subtitles is null.");
                    return;
                }

                let index = subs.findIndex(sub => sub.id === subId);
                if (index === -1) {
                    console.warn("WARN: Subtitle delete event failed, subtitle index is -1.");
                    return;
                }

                subs.splice(index, 1);

                this.player.clearAllSubtitleTracks();
                for (let i = 0; i < subs.length; i++) {
                    const sub = subs[i];
                    this.player.addSubtitle(sub.url, sub.name, sub.shift);
                }

                this.updateRoomSubtitlesHtml(this.currentEntry);
            } break;

            case "subtitleupdate": {
                let data = wsData;
                if (!data) {
                    console.warn("WARN: Subtitle update event failed, event data is null.");
                    return;
                }

                console.info("INFO: Received subtitle update event with:", data);

                let subs = this.currentEntry.subtitles;
                if (!subs) {
                    console.warn("WARN: Subtitle update event failed, currentEntry subtitles is null.");
                    return;
                }

                let index = subs.findIndex(sub => sub.id === data.id);
                if (index === -1) {
                    console.warn("WARN: Subtitle update event failed, subtitle index is -1.");
                    return;
                }

                subs[index].name = data.name;

                this.player.clearAllSubtitleTracks();
                for (let i = 0; i < subs.length; i++) {
                    const sub = subs[i];
                    this.player.addSubtitle(sub.url, sub.name, sub.shift);
                }

                this.updateRoomSubtitlesHtml(this.currentEntry);
            } break;

            case "subtitleattach": {
                let subtitle = wsData;
                console.log(subtitle);
                this.player.addSubtitle(subtitle.url, subtitle.name, subtitle.shift);
                this.player.setToast("Subtitle added: " + subtitle.name);

                if (!this.currentEntry.subtitles) {
                    this.currentEntry.subtitles = [];
                }

                this.currentEntry.subtitles.push(subtitle);
                this.updateRoomSubtitlesHtml(this.currentEntry);
            } break;

            case "subtitleshift": {
                let shift = wsData;

                let subs = this.currentEntry.subtitles;
                for (let i = 0; i < subs.length; i++) {
                    let sub = subs[i];
                    if (sub.id === shift.id) {
                        this.player.setSubtitleShiftByUrl(sub.url, shift.shift);
                        subs[i].shift = shift.shift;
                        break;
                    }
                }
            } break;

            default: {
                console.warn("WARN: Unhandled event of type:", wsType);
            } break;
        }
    }

    shouldPlayNotificationSound(authorId) {
        let messageSoundEnabled = this.settingsMenu.newMessageSoundToggle.classList.contains("active");
        let isAway = this.selected_tab !== TAB_CHAT || document.visibilityState === "hidden";
        let isSelf = this.currentUserId === authorId;
        return messageSoundEnabled && !isSelf && (isAway || this.player.isFullscreen());
    }

    handleDisconnect() {
        this.markAllUsersOffline();
        this.player.setToast("Connection to the server was lost...");
        show(this.connectionLostPopup);
        setTimeout(_ => this.connectToServer(), RECONNECT_AFTER);
    }

    async connectToServer() {
        let success = await this.authenticateAccount(true);
        if (!success) {
            this.handleDisconnect();
            return;
        }

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
