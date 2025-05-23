import * as api from "./api.js";
import { getById, div, a, span, img, svg, button, hide, show } from "./util.js";

export { Playlist }

const ENTRY_ROW_GAP = 4;
const ENTRY_BORDER  = 2;
const ENTRY_HEIGHT  = 64 + ENTRY_BORDER * 2;

const TRANSITION_TIME       = 240;
const BULK_ACTION_DELAY     = 32;
const DRAG_INACTIVITY_DELAY = 32;
const DROPDOWN_EXPAND_TIME  = 100;
const TOUCH_HOLD_DELAY_TIME = 80;

const DESKTOP_PLATFORM_SCROLLING_STEP = ENTRY_HEIGHT * 3.0;
const TOY_TOUCH_DEVICE_SCROLLING_STEP = 32.0

let CONTEXT_MENU_WIDTH  = 180; // hardcoded because .offsetWidth returns 0 when it's hidden
let CONTEXT_MENU_HEIGHT = 204; // hardcoded because it's currently auto

class Playlist {
    constructor() {
        this.controlsNextButton     = getById("playlist_controls_next");
        this.controlsAutoplayButton = getById("playlist_controls_autoplay");
        this.controlsLoopingButton  = getById("playlist_controls_looping");
        this.controlsShuffleButton  = getById("playlist_controls_shuffle");
        this.controlsClearButton    = getById("playlist_controls_clear");
        this.controlsSettingsButton = getById("playlist_controls_settings");

        this.autoplayEnabled = false;
        this.loopingEnabled = false;

        this.htmlEntryList    = getById("playlist_entry_list");
        this.footerEntryCount = getById("playlist_footer_entry_count");

        this.contextMenu           = getById("playlist_context_menu");
        this.contextMenuPlayNow    = getById("context_menu_play_now");
        this.contextMenuMoveTop    = getById("context_menu_move_to_top");
        this.contextMenuMoveBottom = getById("context_menu_move_to_bottom");
        this.contextMenuExpand     = getById("context_menu_expand_entry");
        this.contextMenuEdit       = getById("context_menu_edit");
        this.contextMenuDelete     = getById("context_menu_delete");

        this.contextMenuEntry = null;
        this.contextMenuUserRefactorMe = null;
        this.contextMenuEntryRefactorMe = null;

        // Currently expanded entry. Only one entry is allowed to be expanded at a time.
        this.expandedEntry = null;

        /// State of the entry before the edit began.
            // this.editEntryBefore = null;
        /// Entry that currently is being edited. Only one entry can be edited at a time.
        // this.editEntryNow = null;

        this.isEditingEntry = false;
        this.editEntry = {
            entry: null,
            root:  null,
            title: null,
            url:   null,
        };

        // Corresponds to the actual playlist entries on the server.
        this.entries = [];

        // Represents the structure of the htmlEntryList post transition while entries are still mid transition.
        this.htmlEntries = [];

        this.draggableEntry    = null;
        this.dragStartIndex    = -1;
        this.dragCurrentIndex  = -1;
        this.touchHoldDelay    = null;

        this.draggableEntryMouseOffset = 0; 
        this.shadowedEntryMoveTimout   = null;

        this.scrollIntervalId = null;
        this.scrollingStep = 0;

        this.currentEntryId = 0;
    }

    // NOTE(kihau): Attachable playlist events (similar to the custom player)
    onSettingsClick() {}
    // onClearClick() {}
    // onShuffleClick() {}
    // onLoopingClick() {}
    // onAutoplayClick() {}
    // onPlaylistEntryPlay() {}

    setAutoplay(enabled) {
        if (enabled) {
            this.controlsAutoplayButton.classList.add("active");
        } else {
            this.controlsAutoplayButton.classList.remove("active");
        }

        this.autoplayEnabled = enabled;
    }

    setLooping(enabled) {
        if (enabled) {
            this.controlsLoopingButton.classList.add("active");
        } else {
            this.controlsLoopingButton.classList.remove("active");
        }

        this.loopingEnabled = enabled;
    }

    attachPlaylistEvents() {
        this.controlsNextButton.onclick = _ => {
            if (this.entries.length > 0) {
                api.playerNext(this.currentEntryId);
            }
        }

        this.controlsAutoplayButton.onclick = _ => {
            this.controlsAutoplayButton.classList.toggle("active");
            this.autoplayEnabled = !this.autoplayEnabled;
            api.playerAutoplay(this.autoplayEnabled);
        };

        this.controlsLoopingButton.onclick = _ => {
            this.controlsLoopingButton.classList.toggle("active");
            this.loopingEnabled = !this.loopingEnabled;
            api.playerLooping(this.loopingEnabled);
        };

        this.controlsShuffleButton.onclick  = _ => api.playlistShuffle();
        this.controlsClearButton.onclick    = _ => api.playlistClear();
        this.controlsSettingsButton.onclick = _ => this.onSettingsClick();

        document.addEventListener("click", _ => this.hideContextMenu());

        this.contextMenu.oncontextmenu = event => {
            event.preventDefault();
            this.hideContextMenu();
        };

        this.contextMenuPlayNow.onclick    = _ => this.requestPlaylistPlay(this.contextMenuEntry);
        this.contextMenuMoveTop.onclick    = _ => this.requestEntryMove(this.contextMenuEntry, 0);
        this.contextMenuMoveBottom.onclick = _ => this.requestEntryMove(this.contextMenuEntry, this.entries.length - 1);
        this.contextMenuExpand.onclick     = _ => this.toggleEntryDropdown(this.contextMenuEntry, this.contextMenuEntryRefactorMe, this.contextMenuUserRefactorMe);
        this.contextMenuEdit.onclick       = _ => this.toggleEntryEdit(this.contextMenuEntry, this.contextMenuEntryRefactorMe);
        this.contextMenuDelete.onclick     = _ => this.deleteEntry(this.contextMenuEntry);

        this.htmlEntryList.oncontextmenu = _ => { return false };
    }

    handleUserUpdate(user) {
        for (let i = 0; i < this.entries.length; i++) {
            const entry = this.entries[i];

            if (entry.user_id == user.id) {
                const oldHtmlEntry = this.htmlEntries[i];

                let newHtmlEntry = this.createHtmlEntry(entry, user);
                show(newHtmlEntry);

                this.setEntryPosition(newHtmlEntry, i);
                this.htmlEntries[i] = newHtmlEntry;
                this.htmlEntryList.replaceChild(newHtmlEntry, oldHtmlEntry);
            }
        }
    }

    findUser(users, id) {
        const user = users.find(user => user.id === id);
        if (user) {
            return user;
        } else {
            const dummy = {
                id: 0,
                username: "<Unknown user>",
                avatar: "img/default_avatar.png",
                online: false,
            }

            return dummy;
        }
    }

    updateFooter() {
        this.footerEntryCount.textContent = this.entries.length;
    }

    addEntry(entry, users) {
        this.entries.push(entry);

        const user = this.findUser(users, entry.user_id);
        const htmlEntry = this.createHtmlEntry(entry, user);
        this.setEntryPosition(htmlEntry, this.htmlEntries.length);

        this.htmlEntries.push(htmlEntry);
        this.htmlEntryList.appendChild(htmlEntry);

        window.getComputedStyle(htmlEntry).marginLeft;
        show(htmlEntry);

        this.updateFooter();
    }

    removeAt(index) {
        if (typeof index !== "number") {
            console.error("ERROR: Playlist::removeAt failed. The input index:", index, "is invalid.");
            return;
        }

        if (index < 0 || index >= this.entries.length) {
            console.error("ERROR: Playlist::removeAt failed. Index:", index, "is out of bounds for array:", this.entries);
            return;
        }

        let entry = this.entries[index];
        this.entries.splice(index, 1);

        let htmlEntry = this.htmlEntries[index];
        this.htmlEntries.splice(index, 1);

        hide(htmlEntry);
        setTimeout(_ => this.htmlEntryList.removeChild(htmlEntry), TRANSITION_TIME);

        for (let i = index; i < this.htmlEntries.length; i++) {
            const entry = this.htmlEntries[i];
            this.setEntryPosition(entry, i);
        }

        this.updateFooter();

        return entry;
    }

    move(sourceIndex, destIndex) {
        if (sourceIndex === destIndex) {
            return;
        } else if (destIndex < sourceIndex) {
            for (let i = destIndex; i < sourceIndex; i++) {
                this.setEntryPosition(this.htmlEntries[i], i + 1);
            }
        } else if (destIndex > sourceIndex) {
            for (let i = destIndex; i > sourceIndex; i--) {
                this.setEntryPosition(this.htmlEntries[i], i - 1);
            }
        }

        let htmlEntry = this.htmlEntries[sourceIndex];
        this.htmlEntries.splice(sourceIndex, 1);
        this.htmlEntries.splice(destIndex, 0, htmlEntry);

        let entry = this.entries[sourceIndex];
        this.entries.splice(sourceIndex, 1);
        this.entries.splice(destIndex, 0, entry);

        this.setEntryPosition(htmlEntry, destIndex);
    }

    update(entry, users) {
        for (let i = 0; i < this.entries.length; i++) {
            if (this.entries[i].id === entry.id) {
                const user = this.findUser(users, entry.user_id);
                const htmlEntry = this.createHtmlEntry(entry, user);
                this.setEntryPosition(htmlEntry, i);

                show(htmlEntry);

                if (this.isEditingEntry && entry.id === this.editEntry.entry.id) {
                    this.stopEntryEdit();
                }

                const previous = this.htmlEntries[i];

                this.entries[i] = entry;
                this.htmlEntries[i] = htmlEntry;
                this.htmlEntryList.replaceChild(htmlEntry, previous);
                break;
            }
        }
    }

    loadEntries(entries, users) {
        if (entries === null) {
            console.warn("WARN: Failed to load entries, function input argument is null.");
            return;
        }

        for (let i = 0; i < entries.length; i++) {
            this.entries.push(entries[i]);

            const entry = entries[i];
            const user = this.findUser(users, entry.user_id);
            const htmlEntry = this.createHtmlEntry(entry, user);
            this.setEntryPosition(htmlEntry, this.htmlEntries.length);

            this.htmlEntries.push(htmlEntry);
            this.htmlEntryList.appendChild(htmlEntry);

            setTimeout(_ => {
                window.getComputedStyle(htmlEntry).marginLeft;
                show(htmlEntry);
            }, i * BULK_ACTION_DELAY);
        }

        this.updateFooter();
    }

    clear() {
        // NOTE(kihau): 
        //     This clears the context menu (when inside the entry list) and also the deletion order is random. 
        //     It is generally a bad way to do it!
        const children = this.htmlEntryList.children;
        for (let i = 0; i < children.length; i++) {
            const htmlEntry = children[i];

            setTimeout(_ => {
                hide(htmlEntry);
                setTimeout(_ => this.htmlEntryList.removeChild(htmlEntry), TRANSITION_TIME);
            }, i * BULK_ACTION_DELAY);
        }

        this.htmlEntries   = [];
        this.entries       = [];
        this.expandedEntry = null;

        this.draggableEntry    = null;
        this.dragStartIndex    = -1;
        this.dragCurrentIndex  = -1;

        this.hideContextMenu();
        this.updateFooter();
    }

    findEntryIndex(entry) {
        return this.entries.findIndex(item => item === entry);
    }

    findHtmlIndex(htmlEntry) {
        return this.htmlEntries.findIndex(item => item === htmlEntry);
    }

    expandEntry(htmlEntry, entry, user) {
        if (this.expandedEntry) {
            this.expandedEntry.classList.remove("expand");

            let expanded = this.expandedEntry;
            let dropdown = expanded.getElementsByClassName("playlist_entry_dropdown")[0];
            setTimeout(_ => expanded.removeChild(dropdown), DROPDOWN_EXPAND_TIME);
        }

        if (htmlEntry) {
            let dropdown = this.createEntryDropdown(entry, user);

            this.expandedEntry = htmlEntry;
            this.expandedEntry.appendChild(dropdown);

            window.getComputedStyle(dropdown).height;

            this.expandedEntry.classList.add("expand");
        }
    }

    collapseEntry(htmlEntry) {
        if (htmlEntry !== this.expandedEntry) {
            return;
        }

        if (this.expandedEntry) {
            this.expandedEntry.classList.remove("expand");

            let expanded = this.expandedEntry;
            let dropdown = expanded.getElementsByClassName("playlist_entry_dropdown")[0];
            setTimeout(_ => expanded.removeChild(dropdown), DROPDOWN_EXPAND_TIME);
        }

        this.expandedEntry = null;
    }

    startEntryEdit(entry, root, title, url) {
        this.editEntry.entry = entry;
        this.editEntry.root  = root;
        this.editEntry.title = title;
        this.editEntry.url   = url;

        this.editEntry.root.classList.add("editing");
        this.editEntry.title.contentEditable = true;
        this.editEntry.url.contentEditable   = true;

        this.isEditingEntry = true;
    }

    stopEntryEdit() {
        this.editEntry.root.classList.remove("editing");
        this.editEntry.title.contentEditable = false;
        this.editEntry.url.contentEditable   = false;

        let entry   = this.editEntry.entry;
        entry.title = this.editEntry.title.textContent;
        entry.url   = this.editEntry.url.textContent;

        this.editEntry.entry = null;
        this.editEntry.root  = null;
        this.editEntry.title = null;
        this.editEntry.url   = null;

        this.isEditingEntry = false;

        return entry;
    }

    indexToPosition(index) {
        const rect = this.htmlEntryList.getBoundingClientRect();
        let positionY = rect.y + (ENTRY_HEIGHT + ENTRY_ROW_GAP) * index;
        return positionY;
    }

    positionToIndex(positionY) {
        const rect   = this.htmlEntryList.getBoundingClientRect();
        let index = Math.floor((positionY - rect.y) / (ENTRY_HEIGHT + ENTRY_ROW_GAP));

        if (index < 0) {
            index = 0;
        }

        if (index >= this.htmlEntries.length) {
            index = this.htmlEntries.length - 1;
        }

        return index;
    }

    calculateEntryPosition(index) {
        return index * (ENTRY_HEIGHT + ENTRY_ROW_GAP) + "px";
    }

    setEntryPosition(entry, index) {
        entry.style.top = index * (ENTRY_HEIGHT + ENTRY_ROW_GAP) + "px";
    }

    createEntryDropdown(entry, user) {
        let entryDropdown  = div("playlist_entry_dropdown");
        let proxyLabels    = div("playlist_dropdown_proxy_labels");
        let proxyLabel     = span("playlist_dropdown_proxy_label", "Using proxy");
        let refererLabel   = span("playlist_dropdown_referer_label", "Proxy referer");
        let proxyCheckbox  = /* Checkbox, custom styled */ null;
        let proxyReferer   = a("playlist_dropdown_proxy_referer", entry.referer_url);
        let infoLabels     = div("playlist_dropdown_info_labels");
        let addedByText    = span("playlist_dropdown_added_by", "Added by"); 
        let createdAtText  = span("playlist_dropdown_created_at", "Created at");
        let infoContent    = div("playlist_dropdown_info_content");
        let userAvatar     = div("playlist_dropdown_user_avatar");
        let userAvatarImg  = img(user.avatar);
        let userName       = span("playlist_dropdown_user_name", user.username);
        let date           = new Date(entry.created);
        let creationDate   = span("playlist_dropdown_creation_date", date.toLocaleString());

        entryDropdown.append(infoLabels); {
            infoLabels.append(addedByText);
            infoLabels.append(createdAtText);
        }
        entryDropdown.append(infoContent); {
            infoContent.append(userAvatar); {
                userAvatar.append(userAvatarImg);
            }
            infoContent.append(userName);
            infoContent.append(creationDate);
        }

        return entryDropdown;
    }

    requestPlaylistPlay(htmlEntry) {
        let index = this.htmlEntries.findIndex(item => item === htmlEntry);
        if (index === -1) {
            console.error("ERROR: Failed to find entry:", htmlEntry, "Playlist is out of sync");
            return;
        }

        if (index < 0 || index >= this.entries.length) {
            console.error("ERROR: Delete failed for html playlist entry:", htmlEntry, "Index:", index, "is out of bounds for array:", this.entries);
            return;
        }

        let entry = this.entries[index];
        api.playlistPlay(entry.id, index);
    }

    hideContextMenu() {
        this.contextMenuEntry = null;
        this.contextMenuUserRefactorMe  = null;
        this.contextMenuEntryRefactorMe = null;
        hide(this.contextMenu);
    }

    showContextMenu(event, htmlEntry, entry, user) {
        const entryRect = htmlEntry.getBoundingClientRect();
        const listRect = this.htmlEntryList.getBoundingClientRect();

        let contextMenuX = event.clientX;
        let protrusion = contextMenuX + CONTEXT_MENU_WIDTH - entryRect.right;
        if (protrusion > 0) {
            contextMenuX -= protrusion;
        }

        let contextMenuY = event.clientY;
        protrusion = contextMenuY + CONTEXT_MENU_HEIGHT - listRect.bottom;
        if (protrusion > 0) {
            contextMenuY -= protrusion;
        }

        this.contextMenu.style.left = (contextMenuX - entryRect.left) + "px";
        this.contextMenu.style.top  = (contextMenuY - listRect.top) + "px";
        show(this.contextMenu);

        this.contextMenuEntry = htmlEntry;
        this.contextMenuUserRefactorMe  = user;
        this.contextMenuEntryRefactorMe = entry;
    }

    toggleEntryEdit(htmlEntry, entry) {
        let prevId = 0;
        if (this.isEditingEntry) {
            let newEntry = this.stopEntryEdit();
            prevId = newEntry.id;
            api.playlistUpdate(newEntry);
        } 

        if (prevId !== entry.id) {
            let entryTitle = htmlEntry.getElementsByClassName("playlist_entry_title")[0];
            let entryUrl   = htmlEntry.getElementsByClassName("playlist_entry_url")[0];
            this.startEntryEdit(entry, htmlEntry, entryTitle, entryUrl);
        }
    }

    deleteEntry(htmlEntry) {
        let index = this.htmlEntries.findIndex(item => item === htmlEntry);
        if (index === -1) {
            console.error("ERROR: Failed to find entry:", htmlEntry, "Playlist is out of sync");
            return;
        }

        if (index < 0 || index >= this.entries.length) {
            console.error("ERROR: Delete failed for html playlist entry:", htmlEntry, "Index:", index, "is out of bounds for array:", this.entries);
            return;
        }

        let entry = this.entries[index];
        api.playlistRemove(entry.id, index);
    }

    toggleEntryDropdown(htmlEntry, entry, user) {
        if (this.expandedEntry !== htmlEntry) {
            this.expandEntry(htmlEntry, entry, user);
        } else {
            this.collapseEntry(htmlEntry);
        }
    }

    requestEntryMove(htmlEntry, destIndex) {
        let sourceIndex = this.htmlEntries.findIndex(item => item === htmlEntry);
        if (sourceIndex === -1) {
            console.error("ERROR: Failed to find entry:", htmlEntry, "Playlist is out of sync");
            return;
        }

        if (sourceIndex < 0 || sourceIndex >= this.entries.length) {
            console.error("ERROR: Cannot request playlist move for html playlist entry:", htmlEntry, "Index:", sourceIndex, "is out of bounds for array:", this.entries);
            return;
        }

        let entry = this.entries[sourceIndex];
        let _TODO_handle_this_error = api.playlistMove(entry.id, sourceIndex, destIndex);
        this.move(sourceIndex, destIndex);
    }

    startScrollingUp() {
        if (!this.scrollIntervalId) {
            this.scrollIntervalId = setInterval(_ => this.htmlEntryList.scrollTop -= this.scrollingStep, 16);
        }
    }

    startScrollingDown() {
        if (!this.scrollIntervalId) {
            this.scrollIntervalId = setInterval(_ => this.htmlEntryList.scrollTop += this.scrollingStep, 16);
        }
    }

    stopScrolling() {
        clearInterval(this.scrollIntervalId);
        this.scrollIntervalId = null;
    }

    startEntryDragging(htmlEntry, positionY) {
        this.hideContextMenu();

        this.dragStartIndex   = this.findHtmlIndex(htmlEntry);
        this.dragCurrentIndex = this.dragStartIndex;

        this.collapseEntry(this.expandedEntry);

        let draggableEntry = htmlEntry.cloneNode(true);
        draggableEntry.classList.add("draggable");
        draggableEntry.classList.add("disable_transition");
        this.draggableEntry = draggableEntry;

        this.htmlEntryList.appendChild(draggableEntry);

        let listRect    = this.htmlEntryList.getBoundingClientRect();
        let entryRect   = htmlEntry.getBoundingClientRect();
        let mouseOffset = positionY - entryRect.top;
        let top         = (positionY - listRect.top - mouseOffset) + "px";

        this.draggableEntry.style.top  = top;
        this.draggableEntryMouseOffset = mouseOffset; 

        htmlEntry.classList.add("shadow");
    }

    moveDraggedEntry(positionY) {
        let listRect   = this.htmlEntryList.getBoundingClientRect();
        let top        = positionY - listRect.top - this.draggableEntryMouseOffset;
        let maxPos     = this.indexToPosition(this.htmlEntries.length - 2);
        let maxTop     = Math.min(top, maxPos)

        this.draggableEntry.style.top = maxTop + "px";

        if (positionY - listRect.top < ENTRY_HEIGHT) {
            this.startScrollingUp();
        } else if (positionY - listRect.top > listRect.height - ENTRY_HEIGHT) {
            this.startScrollingDown();
        } else {
            this.stopScrolling();
        }

        clearTimeout(this.shadowedEntryMoveTimout);
        this.shadowedEntryMoveTimout = setTimeout(_ => this.moveShadowedEntry(), DRAG_INACTIVITY_DELAY);
    }

    stopEntryDragging(htmlEntry) {
        clearTimeout(this.shadowedEntryMoveTimout);
        this.moveShadowedEntry();
        this.stopScrolling();


        let entryRect = this.draggableEntry.getBoundingClientRect();
        let listRect  = this.htmlEntryList.getBoundingClientRect();
        let listScroll = this.htmlEntryList.scrollTop;
        let entryTop = entryRect.top - listRect.top + listScroll;


        this.draggableEntry.classList.remove("draggable");
        this.draggableEntry.style.top = entryTop + "px";

        window.getComputedStyle(this.draggableEntry).top;
        this.draggableEntry.classList.remove("disable_transition");

        let oldPos = this.draggableEntry.style.top;
        let newPos = this.calculateEntryPosition(this.dragCurrentIndex);

        if (oldPos === newPos) {
            this.htmlEntryList.removeChild(this.draggableEntry);
            htmlEntry.classList.remove("shadow");
        } else {
            this.setEntryPosition(this.draggableEntry, this.dragCurrentIndex);
            this.draggableEntry.ontransitionend = event => {
                this.htmlEntryList.removeChild(event.target);
                htmlEntry.classList.remove("shadow");
            };
        }
        this.draggableEntry            = null;
        this.draggableEntryMouseOffset = 0; 


        if (this.dragStartIndex !== this.dragCurrentIndex) {
            let entry = this.entries[this.dragCurrentIndex];
            api.playlistMove(entry.id, this.dragStartIndex, this.dragCurrentIndex);
        }
    }

    moveShadowedEntry() {
        let listScroll  = this.htmlEntryList.scrollTop;
        let dragRect = this.draggableEntry.getBoundingClientRect();
        let dragPos  = dragRect.y + listScroll + ENTRY_HEIGHT / 2.0;

        let hoverIndex = this.positionToIndex(dragPos);
        let startIndex = this.dragCurrentIndex;
        let endIndex   = this.dragCurrentIndex;

        if (hoverIndex < this.dragCurrentIndex) {
            for (let i = this.dragCurrentIndex - 1; i >= 0; i--) {
                const pos = this.indexToPosition(i);
                if (dragPos > pos + ENTRY_HEIGHT * 0.66666) {
                    break;
                }

                endIndex -= 1;

                const entry = this.htmlEntries[i];
                this.setEntryPosition(entry, i + 1);
            }
        } else if (hoverIndex > this.dragCurrentIndex) {
            for (let i = this.dragCurrentIndex + 1; i < this.htmlEntries.length; i++) {
                const pos = this.indexToPosition(i);
                if (dragPos < pos + ENTRY_HEIGHT * 0.33333) {
                    break;
                }

                endIndex += 1;

                const entry = this.htmlEntries[i];
                this.setEntryPosition(entry, i - 1);
            }
        }

        if (startIndex === endIndex) {
            return;
        }

        let entry = this.htmlEntries[startIndex];
        this.htmlEntries.splice(startIndex, 1);
        this.htmlEntries.splice(endIndex, 0, entry);

        { // NOTE(kihau): This will be handled differently.
            let entry = this.entries[startIndex];
            this.entries.splice(startIndex, 1);
            this.entries.splice(endIndex, 0, entry);
        }

        this.setEntryPosition(entry, endIndex);
        this.dragCurrentIndex = endIndex;
    }

    createHtmlEntry(entry, user) {
        let entryRoot      = div("playlist_entry");
        let entryTop       = div("playlist_entry_top"); 
        let entryDragArea  = div("playlist_drag_area"); 
        let entryThumbnail = div("playlist_entry_thumbnail");
        let thumbnailSrc   = entry.thumbnail ? entry.thumbnail : "img/thumbnail_placeholder.png";
        let thumbnailPlay  = svg("svg/main_icons.svg#thumbnail_play")
        let thumbnailImg   = img(thumbnailSrc);
        let entryInfo      = div("playlist_entry_info");
        let entryTitle     = span("playlist_entry_title", entry.title);
        let entryUrl       = a("playlist_entry_url", entry.url);
        let entryButtons   = div("playlist_entry_buttons");
        let editButton     = button("playlist_entry_edit_button", "Edit playlist entry");
        let editSvg        = svg("svg/main_icons.svg#edit");
        let deleteButton   = button("playlist_entry_delete_button", "Delete playlist entry");
        let deleteSvg      = svg("svg/main_icons.svg#delete");
        let dropdownButton = div("playlist_dropdown_button");
        let dropdownSvg    = svg("svg/main_icons.svg#dropdown");

        //
        // Setting html elements content.
        //
        entryDragArea.textContent = "☰";

        //
        // Attaching events to html elements.
        //

        // Scrolling on screen touch.
        entryRoot.ontouchstart = _ => {};

        entryRoot.oncontextmenu = event => {
            event.preventDefault();

            if (this.contextMenuEntry === entryRoot) {
                this.hideContextMenu();
            } else {
                this.showContextMenu(event, entryRoot, entry, user);
            }
        };

        entryDragArea.oncontextmenu = event => {
            event.preventDefault();
            event.stopPropagation();
        };

        entryDragArea.ontouchstart = event => {
            clearTimeout(this.touchHoldDelay);
            this.touchHoldDelay = setTimeout(_ => {
                this.scrollingStep = TOY_TOUCH_DEVICE_SCROLLING_STEP;
                this.htmlEntryList.classList.add("disable-smooth-scrolling-for-toy-touch-device-systems");
                this.startEntryDragging(entryRoot, event.touches[0].clientY);

                let onDragging = event => {
                    this.moveDraggedEntry(event.touches[0].clientY);
                    event.preventDefault();
                    event.stopPropagation();
                };

                let onDraggingStop = _ => {
                    this.stopEntryDragging(entryRoot);
                    document.removeEventListener("touchmove", onDragging);
                    document.removeEventListener("touchend",  onDraggingStop);
                };

                document.addEventListener("touchmove", onDragging, { passive: false });
                document.addEventListener("touchend",  onDraggingStop);
            }, TOUCH_HOLD_DELAY_TIME);
        };

        entryDragArea.ontouchmove = _ => clearTimeout(this.touchHoldDelay);
        entryDragArea.ontouchend  = _ => clearTimeout(this.touchHoldDelay);

        entryDragArea.onmousedown = event => {
            this.scrollingStep = DESKTOP_PLATFORM_SCROLLING_STEP;
            this.htmlEntryList.classList.remove("disable-smooth-scrolling-for-toy-touch-device-systems");
            this.startEntryDragging(entryRoot, event.clientY);

            let onDragging = event => {
                this.moveDraggedEntry(event.clientY);
            };

            let onDraggingStop = _ => {
                this.stopEntryDragging(entryRoot);

                document.removeEventListener("mousemove", onDragging);
                document.removeEventListener("mouseup",   onDraggingStop);
            };

            document.addEventListener("mousemove", onDragging);
            document.addEventListener("mouseup",   onDraggingStop);
        };


        entryThumbnail.onclick = _ => this.requestPlaylistPlay(entryRoot);
        editButton.onclick     = _ => this.toggleEntryEdit(entryRoot, entry);
        deleteButton.onclick   = _ => this.deleteEntry(entryRoot);
        dropdownButton.onclick = _ => this.toggleEntryDropdown(entryRoot, entry, user);

        //
        // Constructing html element structure.
        //
        entryRoot.append(entryTop); {
            entryTop.append(entryDragArea);
            entryTop.append(entryThumbnail); {
                entryThumbnail.append(thumbnailImg);
                entryThumbnail.append(thumbnailPlay);
            }
            entryTop.append(entryInfo); {
                entryInfo.append(entryTitle);
                entryInfo.append(entryUrl);
            }
            entryTop.append(entryButtons); {
                entryButtons.append(editButton); {
                    editButton.append(editSvg);
                }
                entryButtons.append(deleteButton); {
                    deleteButton.append(deleteSvg);
                }
            }
            entryTop.append(dropdownButton); {
                dropdownButton.append(dropdownSvg);
            }
        }

        return entryRoot;
    }

    handleServerEvent(action, data, users) {
        switch (action) {
            case "add": {
                this.addEntry(data, users);
            } break;

            case "addmany": {
                this.loadEntries(data, users);
            } break;

            case "clear": {
                this.clear();
            } break;

            case "remove": {
                this.removeAt(data)
            } break;

            case "shuffle": {
                this.clear();
                this.loadEntries(data, users);
            } break;

            case "move": {
                this.move(data.source_index, data.dest_index);
            } break;

            case "update": {
                this.update(data, users);
            } break;

            default: {
                console.error("Unknown playlist action:", action, "with data:", data);
            } break;
        }
    }
}
