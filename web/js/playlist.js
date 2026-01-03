import * as api from "./api.js";
import * as common from "./common.js";
import { getById, div, a, span, img, svg, button, hide, show, isScrollableVisible, getCssNumber } from "./util.js";

export { Playlist }

// NOTE(kihau): Adjustable row gap value. No need to update CSS.
const ENTRY_ROW_GAP = 4;

// NOTE(kihau): Hardcoded size values from the playlist.css. CSS needs to be updated.
const ENTRY_BORDER  = getCssNumber("--playlist_entry_border", "px");
const ENTRY_HEIGHT  = getCssNumber("--playlist_entry_height", "px") + ENTRY_BORDER * 2;
const ENTRY_TRANSITION_TIME = getCssNumber("--playlist_entry_transition_time", "ms");

const BULK_ACTION_DELAY     = 32;
const DRAG_INACTIVITY_DELAY = 32;
const TOUCH_HOLD_DELAY_TIME = 80;

const SCROLLING_STEP = ENTRY_HEIGHT / 3.0;

class Playlist {
    constructor() {
        this.controlsNextButton     = getById("playlist_controls_next");
        this.controlsAutoplayButton = getById("playlist_controls_autoplay");
        this.controlsLoopingButton  = getById("playlist_controls_looping");
        this.controlsShuffleButton  = getById("playlist_controls_shuffle");
        this.controlsClearButton    = getById("playlist_controls_clear");
        this.controlsSettingsButton = getById("playlist_controls_settings");

        this.autoplayEnabled = false;
        this.loopingEnabled  = false;

        this.htmlEntryListRoot = getById("playlist_entry_list_root");
        this.htmlEntryList     = getById("playlist_entry_list");
        this.footerEntryCount  = getById("playlist_footer_entry_count");

        this.contextMenu           = getById("playlist_context_menu");
        this.contextMenuPlayNow    = getById("playlist_context_play_now");
        this.contextMenuMoveTop    = getById("playlist_context_move_to_top");
        this.contextMenuMoveBottom = getById("playlist_context_move_to_bottom");
        this.contextMenuExpand     = getById("playlist_context_expand_entry");
        this.contextMenuExpandText = getById("playlist_context_expand_entry_text");
        this.contextMenuCopyUrl    = getById("playlist_context_copy_url");
        this.contextMenuEdit       = getById("playlist_context_edit");
        this.contextMenuDelete     = getById("playlist_context_delete");

        this.contextMenuEntry     = null;
        this.contextMenuUser      = null;
        this.contextMenuHtmlEntry = null;

        // Currently expanded entry. Only one entry is allowed to be expanded at a time.
        this.expandedEntry = null;

        this.isEditingEntry = false;
        this.editEntry = {
            entry: null,
            root:  null,
            title: null,
            url:   null,
        };

        // Corresponds to the actual playlist entries on the server.
        this.entries = [];

        // Represents the structure of the htmlEntryList post transition while entries are still mid-transition.
        this.htmlEntries = [];

        this.draggableEntry    = null;
        this.dragStartIndex    = -1;
        this.dragCurrentIndex  = -1;
        this.touchHoldDelay    = null;

        this.draggableEntryMouseOffset = 0; 
        this.shadowedEntryMoveTimout   = null;

        this.scrollIntervalId = null;
        this.currentEntryId = 0;
    }

    // NOTE(kihau): Attachable playlist events (similar to the custom player)
    onSettingsClick() {}

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
                api.wsPlayerNext(this.currentEntryId);
            }
        };

        this.controlsAutoplayButton.onclick = _ => {
            this.controlsAutoplayButton.classList.toggle("active");
            this.autoplayEnabled = !this.autoplayEnabled;
            api.wsPlayerAutoplay(this.autoplayEnabled);
        };

        this.controlsLoopingButton.onclick = _ => {
            this.controlsLoopingButton.classList.toggle("active");
            this.loopingEnabled = !this.loopingEnabled;
            api.wsPlayerLooping(this.loopingEnabled);
        };

        this.controlsShuffleButton.onclick  = _ => api.wsPlaylistShuffle();
        this.controlsClearButton.onclick    = _ => api.wsPlaylistClear();
        this.controlsSettingsButton.onclick = _ => this.onSettingsClick();

        document.addEventListener("click", _ => this.hideContextMenu());

        this.contextMenu.oncontextmenu = event => {
            event.preventDefault();
            this.hideContextMenu();
        };

        this.contextMenuPlayNow.onclick    = _ => api.wsPlaylistPlay(this.contextMenuEntry.id);
        this.contextMenuMoveTop.onclick    = _ => api.wsPlaylistMove(this.contextMenuEntry.id, 0);
        this.contextMenuMoveBottom.onclick = _ => api.wsPlaylistMove(this.contextMenuEntry.id, this.entries.length - 1);
        this.contextMenuExpand.onclick     = _ => this.toggleEntryDropdown(this.contextMenuHtmlEntry, this.contextMenuEntry, this.contextMenuUser);
        this.contextMenuCopyUrl.onclick    = _ => navigator.clipboard.writeText(this.contextMenuEntry.url);
        this.contextMenuEdit.onclick       = _ => this.toggleEntryEdit(this.contextMenuHtmlEntry, this.contextMenuEntry);
        this.contextMenuDelete.onclick     = _ => api.wsPlaylistDelete(this.contextMenuEntry.id);

        this.htmlEntryList.oncontextmenu = _ => { return false };
    }

    handleUserUpdate(user) {
        for (let i = 0; i < this.entries.length; i++) {
            const entry = this.entries[i];

            if (entry.user_id === user.id) {
                const oldHtmlEntry = this.htmlEntries[i];

                let newHtmlEntry = this.createHtmlEntry(entry, user);
                show(newHtmlEntry);

                if (this.dragCurrentIndex === i) {
                    newHtmlEntry.classList.add("shadow");
                }

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
            };

            return dummy;
        }
    }

    updateFooter() {
        this.footerEntryCount.textContent = String(this.entries.length);
    }

    // This method updates min-height on the HTML entry list. 
    // Because all child elements have position absolute, HTML entry list won't get its height updated automatically,
    // which is why the updated must be performed manually...
    updateHtmlListHeight() {
        let height = (ENTRY_HEIGHT + ENTRY_ROW_GAP) * this.entries.length - ENTRY_ROW_GAP;
        this.htmlEntryList.style.minHeight = height + "px";
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

        this.updateHtmlListHeight();
        this.updateFooter();
    }

    addEntryTop(entry, users) {
        if (this.draggableEntry) {
            this.dragStartIndex += 1;
            this.dragCurrentIndex += 1;
        }

        this.entries.unshift(entry);

        for (let i = 0; i < this.htmlEntries.length; i++) {
            const entry = this.htmlEntries[i];
            this.setEntryPosition(entry, i + 1);
        }

        const user = this.findUser(users, entry.user_id);
        const htmlEntry = this.createHtmlEntry(entry, user);
        this.setEntryPosition(htmlEntry, 0);

        this.htmlEntries.unshift(htmlEntry);
        this.htmlEntryList.appendChild(htmlEntry);

        window.getComputedStyle(htmlEntry).marginLeft;
        show(htmlEntry);

        this.updateHtmlListHeight();
        this.updateFooter();
    }

   delete(entryId) {
        let index = this.entries.findIndex(item => item.id === entryId);
        if (index === -1) {
            console.error("ERROR: Playlist::delete failed. Entry with id", entryId, "is not in the playlist.");
            return null;
        }

        if (this.contextMenuEntry && entryId === this.contextMenuEntry.id) {
            this.hideContextMenu();
        }

        if (this.draggableEntry) {
            if (index === this.dragCurrentIndex) {
                index = this.dragStartIndex;
                this.cancelEntryDragging();
            } else  {
                if (index < this.dragStartIndex) {
                    this.dragStartIndex -= 1;
                }

                if (index < this.dragCurrentIndex) {
                    this.dragCurrentIndex -= 1;
                }
            }
        }

        let entry = this.entries[index];
        this.entries.splice(index, 1);

        let htmlEntry = this.htmlEntries[index];
        this.htmlEntries.splice(index, 1);

        hide(htmlEntry);
        setTimeout(_ => this.htmlEntryList.removeChild(htmlEntry), ENTRY_TRANSITION_TIME);

        for (let i = index; i < this.htmlEntries.length; i++) {
            const entry = this.htmlEntries[i];
            this.setEntryPosition(entry, i);
        }

        this.updateHtmlListHeight();
        this.updateFooter();
        return entry;
    }

    internalMove(sourceIndex, destIndex) {
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

    swap(sourceIndex, destIndex) {
        let entry = this.entries[sourceIndex];
        this.entries[sourceIndex] = this.entries[destIndex];
        this.entries[destIndex] = entry;

        let htmlEntry = this.htmlEntries[sourceIndex];
        this.htmlEntries[sourceIndex] = this.htmlEntries[destIndex];
        this.htmlEntries[destIndex] = htmlEntry;
    }

    revertDragging(sourceIndex, destIndex) {
        let htmlEntry = this.htmlEntries[sourceIndex];
        this.htmlEntries.splice(sourceIndex, 1);
        this.htmlEntries.splice(destIndex, 0, htmlEntry);

        let entry = this.entries[sourceIndex];
        this.entries.splice(sourceIndex, 1);
        this.entries.splice(destIndex, 0, entry);
    }

    move(entryId, destIndex) {
        let index = this.entries.findIndex(item => item.id === entryId);
        if (index === -1) {
            console.error("ERROR: Playlist::move failed. Provided entry with ID:", entryId, "is not within the playlist entry list.");
            return;
        }

        if (this.draggableEntry) {
            if (index === this.dragCurrentIndex) {
                index = this.dragStartIndex;
                this.cancelEntryDragging();
            } else {
                // NOTE(kihau): Entry Id -> Destination approach
                if (destIndex >= this.dragStartIndex && destIndex <= this.dragCurrentIndex) {
                    if (destIndex !== this.dragStartIndex || index <= this.dragStartIndex) {
                        destIndex -= 1;
                    } 
                } else if (destIndex <= this.dragStartIndex && destIndex >= this.dragCurrentIndex) {
                    if (destIndex !== this.dragStartIndex || index >= this.dragStartIndex) {
                        destIndex += 1;
                    } 
                }

                if (index > this.dragStartIndex && destIndex <= this.dragStartIndex) {
                    this.dragStartIndex += 1;
                } else if (index < this.dragStartIndex && destIndex >= this.dragStartIndex) {
                    this.dragStartIndex -= 1;
                }

                if (index > this.dragCurrentIndex && destIndex <= this.dragCurrentIndex) {
                    this.dragCurrentIndex += 1;
                } else if (index < this.dragCurrentIndex && destIndex >= this.dragCurrentIndex) {
                    this.dragCurrentIndex -= 1;
                }
            }
        }

        this.internalMove(index, destIndex);
    }

    update(entry, users) {
        let index = this.entries.findIndex(item => item.id === entry.id);
        if (index === -1) {
            console.error("ERROR: Playlist::update failed. Provided entry with ID:", entry.id, "is not within the playlist entry list.");
            return;
        }

        const user = this.findUser(users, entry.user_id);
        const htmlEntry = this.createHtmlEntry(entry, user);
        this.setEntryPosition(htmlEntry, index);
        show(htmlEntry);

        const entryBefore = this.htmlEntries[index];
        this.htmlEntryList.replaceChild(htmlEntry, entryBefore);

        if (this.isEditingEntry && entry.id === this.editEntry.entry.id) {
            this.stopEntryEdit();
        }

        if (this.draggableEntry && this.dragCurrentIndex === index) {
            let draggableBefore = this.draggableEntry;
            this.draggableEntry = this.createDraggableEntry(htmlEntry, draggableBefore.style.top);
            this.htmlEntryList.replaceChild(this.draggableEntry, draggableBefore);
            htmlEntry.classList.add("shadow");
        }

        this.entries[index]     = entry;
        this.htmlEntries[index] = htmlEntry;
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

            if (isScrollableVisible(this.htmlEntryListRoot, htmlEntry)) {
                setTimeout(_ => {
                    window.getComputedStyle(htmlEntry).marginLeft;
                    show(htmlEntry);
                }, i * BULK_ACTION_DELAY);
            } else {
                show(htmlEntry);
            }
        }

        this.updateHtmlListHeight();
        this.updateFooter();
    }

    loadEntriesTop(entries, users) {
        if (entries === null) {
            console.warn("WARN: Failed to load entries, function input argument is null.");
            return;
        }

        if (this.draggableEntry) {
            this.dragStartIndex += entries.length;
            this.dragCurrentIndex += entries.length;
        }

        for (let i = 0; i < this.htmlEntries.length; i++) {
            const entry = this.htmlEntries[i];
            this.setEntryPosition(entry, entries.length + i);
        }

        for (let i = entries.length - 1; i >= 0; i--) {
            this.entries.unshift(entries[i]);

            const entry = entries[i];
            const user = this.findUser(users, entry.user_id);
            const htmlEntry = this.createHtmlEntry(entry, user);
            this.setEntryPosition(htmlEntry, i);

            this.htmlEntries.unshift(htmlEntry);
            this.htmlEntryList.appendChild(htmlEntry);

            if (isScrollableVisible(this.htmlEntryListRoot, htmlEntry)) {
                setTimeout(_ => {
                    window.getComputedStyle(htmlEntry).marginLeft;
                    show(htmlEntry);
                }, i * BULK_ACTION_DELAY);
            } else {
                show(htmlEntry);
            }
        }

        this.updateHtmlListHeight();
        this.updateFooter();
    }

    clear() {
        this.cancelEntryDragging();
        this.hideContextMenu();

        for (let i = 0; i < this.htmlEntries.length; i++) {
            const htmlEntry = this.htmlEntries[i];

            if (isScrollableVisible(this.htmlEntryListRoot, htmlEntry)) {
                setTimeout(_ => {
                    hide(htmlEntry);
                    setTimeout(_ => this.htmlEntryList.removeChild(htmlEntry), ENTRY_TRANSITION_TIME);
                }, i * BULK_ACTION_DELAY);
            } else {
                this.htmlEntryList.removeChild(htmlEntry)
            }
        }

        this.htmlEntries   = [];
        this.entries       = [];

        this.updateHtmlListHeight();
        this.updateFooter();
    }

    findHtmlIndex(htmlEntry) {
        return this.htmlEntries.findIndex(item => item === htmlEntry);
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
        const rect = this.htmlEntryListRoot.getBoundingClientRect();
        let positionY = rect.y + (ENTRY_HEIGHT + ENTRY_ROW_GAP) * index;
        return positionY;
    }

    positionToIndex(positionY) {
        const rect   = this.htmlEntryListRoot.getBoundingClientRect();
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


    hideContextMenu() {
        if (this.contextMenuHtmlEntry) {
            this.contextMenuHtmlEntry.classList.remove("highlight");
        }

        this.contextMenuHtmlEntry = null;
        this.contextMenuUser  = null;
        this.contextMenuEntry = null;
        hide(this.contextMenu);
    }

    showContextMenu(event, htmlEntry, entry, user) {
        if (this.contextMenuHtmlEntry) {
            this.contextMenuHtmlEntry.classList.remove("highlight");
        }

        show(this.contextMenu);

        if (this.expandedEntry === htmlEntry) {
            this.contextMenuExpandText.textContent = "Collapse";
            this.contextMenuExpand.classList.add("expanded");
        } else {
            this.contextMenuExpandText.textContent = "Expand";
            this.contextMenuExpand.classList.remove("expanded");
        }

        const entryRect = htmlEntry.getBoundingClientRect();
        const rootRect  = this.htmlEntryListRoot.getBoundingClientRect();
        const listRect  = this.htmlEntryList.getBoundingClientRect();
        const height    = this.contextMenu.offsetHeight;
        const width     = this.contextMenu.offsetWidth;

        let contextMenuX = event.clientX;
        let protrusion = contextMenuX + width - entryRect.right;
        if (protrusion > 0) {
            contextMenuX -= protrusion;
        }

        let contextMenuY = event.clientY;
        protrusion = contextMenuY + height - rootRect.bottom;
        if (protrusion > 0) {
            contextMenuY -= protrusion;
        }

        this.contextMenu.style.left = (contextMenuX - entryRect.left) + "px";
        this.contextMenu.style.top  = (contextMenuY - listRect.top)   + "px";

        this.contextMenuEntry     = entry;
        this.contextMenuHtmlEntry = htmlEntry;
        this.contextMenuUser      = user;

        this.contextMenuHtmlEntry.classList.add("highlight");
    }

    toggleEntryEdit(htmlEntry, entry) {
        let prevId = 0;
        if (this.isEditingEntry) {
            let newEntry = this.stopEntryEdit();
            prevId = newEntry.id;
            api.wsPlaylistUpdate(newEntry);
        } 

        if (prevId !== entry.id) {
            let entryTitle = htmlEntry.getElementsByClassName("playlist_entry_title")[0];
            let entryUrl   = htmlEntry.getElementsByClassName("playlist_entry_url")[0];
            this.startEntryEdit(entry, htmlEntry, entryTitle, entryUrl);
        }
    }

    startScrollingUp() {
        if (!this.scrollIntervalId) {
            this.scrollIntervalId = setInterval(_ => this.htmlEntryListRoot.scrollTop -= SCROLLING_STEP, 16);
        }
    }

    startScrollingDown() {
        if (!this.scrollIntervalId) {
            this.scrollIntervalId = setInterval(_ => this.htmlEntryListRoot.scrollTop += SCROLLING_STEP, 16);
        }
    }

    stopScrolling() {
        clearInterval(this.scrollIntervalId);
        this.scrollIntervalId = null;
    }

    createDraggableEntry(htmlEntry, positionTop) {
        let draggableEntry = htmlEntry.cloneNode(true);
        draggableEntry.classList.add("draggable");
        draggableEntry.classList.add("disable_transition");
        draggableEntry.style.top = positionTop;
        return draggableEntry;
    }

    startEntryDragging(htmlEntry, positionY) {
        this.hideContextMenu();
        common.collapseEntry(this, this.expandedEntry);

        let listRect    = this.htmlEntryListRoot.getBoundingClientRect();
        let entryRect   = htmlEntry.getBoundingClientRect();
        let mouseOffset = positionY - entryRect.top;
        let top         = (positionY - listRect.top - mouseOffset) + "px";

        this.dragStartIndex   = this.findHtmlIndex(htmlEntry);
        this.dragCurrentIndex = this.dragStartIndex;
        this.draggableEntry   = this.createDraggableEntry(htmlEntry, top);
        this.htmlEntryList.appendChild(this.draggableEntry);

        this.draggableEntry.style.top  = top;
        this.draggableEntryMouseOffset = mouseOffset; 

        htmlEntry.classList.add("shadow");
    }

    moveDraggedEntry(positionY) {
        if (!this.draggableEntry) {
            return;
        }

        let listRect   = this.htmlEntryListRoot.getBoundingClientRect();
        let top        = positionY - listRect.top - this.draggableEntryMouseOffset;
        let maxPos     = this.indexToPosition(this.htmlEntries.length - 2);
        let maxTop     = Math.min(top, maxPos);

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

    stopEntryDragging() {
        if (!this.draggableEntry) {
            return;
        }

        clearTimeout(this.shadowedEntryMoveTimout);
        this.moveShadowedEntry();
        this.stopScrolling();

        let entryRect  = this.draggableEntry.getBoundingClientRect();
        let listRect   = this.htmlEntryListRoot.getBoundingClientRect();
        let listScroll = this.htmlEntryListRoot.scrollTop;
        let entryTop   = entryRect.top - listRect.top + listScroll;

        this.draggableEntry.classList.remove("draggable");
        this.draggableEntry.style.top = entryTop + "px";

        window.getComputedStyle(this.draggableEntry).top;
        this.draggableEntry.classList.remove("disable_transition");

        let oldPos = this.draggableEntry.style.top;
        let newPos = this.calculateEntryPosition(this.dragCurrentIndex);

        let htmlEntry = this.htmlEntries[this.dragCurrentIndex];

        if (oldPos === newPos) {
            this.htmlEntryList.removeChild(this.draggableEntry);
            htmlEntry.classList.remove("shadow");
        } else {
            this.setEntryPosition(this.draggableEntry, this.dragCurrentIndex);

            let draggable = this.draggableEntry;
            setTimeout(_ => {
                htmlEntry.classList.remove("shadow");
                if (this.htmlEntryList.contains(draggable)) {
                    this.htmlEntryList.removeChild(draggable);
                }
            }, ENTRY_TRANSITION_TIME);
        }

        if (this.dragStartIndex !== this.dragCurrentIndex) {
            let entry = this.entries[this.dragCurrentIndex];
            this.revertDragging(this.dragCurrentIndex, this.dragStartIndex);
            api.wsPlaylistMove(entry.id, this.dragCurrentIndex);
        }

        this.draggableEntry            = null;
        this.draggableEntryMouseOffset = 0; 
    }

    cancelEntryDragging() {
        if (!this.draggableEntry) {
            return;
        }

        clearTimeout(this.shadowedEntryMoveTimout);
        this.stopScrolling();

        this.htmlEntryList.removeChild(this.draggableEntry);

        let htmlEntry = this.htmlEntries[this.dragCurrentIndex];
        htmlEntry.classList.remove("shadow");

        this.internalMove(this.dragCurrentIndex, this.dragStartIndex);

        this.draggableEntry    = null;
        this.dragStartIndex    = -1;
        this.dragCurrentIndex  = -1;
        this.draggableEntryMouseOffset = 0; 
    }

    moveShadowedEntry() {
        if (!this.draggableEntry) {
            return;
        }

        let listScroll = this.htmlEntryListRoot.scrollTop;
        let dragRect   = this.draggableEntry.getBoundingClientRect();
        let dragPos    = dragRect.y + listScroll + ENTRY_HEIGHT / 2.0;

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

        let htmlEntry = this.htmlEntries[startIndex];
        this.htmlEntries.splice(startIndex, 1);
        this.htmlEntries.splice(endIndex, 0, htmlEntry);

        let entry = this.entries[startIndex];
        this.entries.splice(startIndex, 1);
        this.entries.splice(endIndex, 0, entry);

        this.setEntryPosition(htmlEntry, endIndex);
        this.dragCurrentIndex = endIndex;
    }

    createHtmlEntry(entry, user) {
        let entryRoot      = div("playlist_entry");
        let entryTop       = div("playlist_entry_top"); 
        let entryDragArea  = div("playlist_drag_area"); 
        let entryThumbnail = div("playlist_entry_thumbnail");
        let thumbnailSrc   = entry.thumbnail ? entry.thumbnail : "img/thumbnail_placeholder.png";
        let thumbnailPlay  = svg("svg/main_icons.svg#thumbnail_play");
        let thumbnailImg   = img(thumbnailSrc, true);
        let entryInfo      = div("playlist_entry_info");
        let entryTitle     = span("playlist_entry_title", entry.title);
        let entryUrl       = a("playlist_entry_url", entry.url);
        let entryButtons   = div("playlist_entry_buttons");
        let editButton     = button("playlist_entry_edit_button", "Edit playlist entry");
        let editSvg        = svg("svg/main_icons.svg#edit");
        let deleteButton   = button("playlist_entry_delete_button", "Delete playlist entry");
        let deleteSvg      = svg("svg/main_icons.svg#delete");
        let dropdownButton = div("entry_dropdown_button");
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

            if (this.contextMenuHtmlEntry === entryRoot) {
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
                this.startEntryDragging(entryRoot, event.touches[0].clientY);

                let onDragging = event => {
                    this.moveDraggedEntry(event.touches[0].clientY);
                    event.preventDefault();
                    event.stopPropagation();
                };

                let onDraggingStop = _ => {
                    this.stopEntryDragging();
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
            this.startEntryDragging(entryRoot, event.clientY);

            let onDragging = event => {
                this.moveDraggedEntry(event.clientY);
            };

            let onDraggingStop = _ => {
                this.stopEntryDragging();

                document.removeEventListener("mousemove", onDragging);
                document.removeEventListener("mouseup",   onDraggingStop);
            };

            document.addEventListener("mousemove", onDragging);
            document.addEventListener("mouseup",   onDraggingStop);
        };

        entryTitle.onkeydown = event => {
            if (event.key === "Enter" && this.isEditingEntry) {
                this.toggleEntryEdit(entryRoot, entry)
            }
        };

        entryUrl.onkeydown = event => {
            if (event.key === "Enter" && this.isEditingEntry) {
                this.toggleEntryEdit(entryRoot, entry)
            }
        };

        entryThumbnail.onclick = _ => api.wsPlaylistPlay(entry.id);
        editButton.onclick     = _ => this.toggleEntryEdit(entryRoot, entry);
        deleteButton.onclick   = _ => api.wsPlaylistDelete(entry.id);
        dropdownButton.onclick = _ => common.toggleEntryDropdown(this, entryRoot, entry, user);

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

            case "addtop": {
                this.addEntryTop(data, users);
            } break;

            case "addmany": {
                this.loadEntries(data, users);
            } break;

            case "addmanytop": {
                this.loadEntriesTop(data, users);
            } break;

            case "clear": {
                this.clear();
            } break;

            case "delete": {
                this.delete(data)
            } break;

            case "shuffle": {
                this.clear();
                this.loadEntries(data, users);
            } break;

            case "move": {
                this.move(data.entry_id, data.dest_index);
            } break;

            case "update": {
                this.update(data, users);
            } break;

            default: {
                console.error("ERROR: Unknown playlist action:", action, "with data:", data);
            } break;
        }
    }
}
