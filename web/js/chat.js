import * as api from "./api.js";
import { getById, div, img, span, a, isSameDay, isLocalImage, show, hide, clearContent } from "./util.js";

export { Chat }

const CHARACTER_LIMIT = 1000;

class Chat {
    constructor() {
        this.contentRoot         = getById("content_chat");
        this.chatInput           = getById("chat_input_box");
        this.chatListRoot        = getById("chat_message_list_root");
        this.chatList            = getById("chat_message_list");
        this.sendMessageButton   = getById("chat_input_send_button");
        this.uploadButton        = getById("chat_input_upload_button");
        this.uploadImageInput    = getById("chat_input_upload_image");
        this.contextMenu         = getById("chat_context_menu");
        this.contextMenuEdit     = getById("chat_context_edit");
        this.contextMenuDelete   = getById("chat_context_delete");
        this.contextMenuCopy     = getById("chat_context_copy");
        this.contextMenuCopyUrl  = getById("chat_context_copy_url");
        this.contextMenuOpen     = getById("chat_context_open");

        this.currentUserId = -1;

        this.prevUserId     = -1;
        this.prevDate       = new Date();

        // NOTE(kihau): Maximum of 200-1000 html message elements will be within the chatList DOM at any given time.
        // NOTE(kihau): Maybe those could be ring buffers with 10000 messages?
        this.messages     = [];
        this.htmlMessages = [];

        this.contextMenuMessage     = null;
        this.contextMenuHtmlMessage = null;
        this.contextMenuUrl         = "";
        this.contextMenuShowUrl     = false;

        this.editingMessage     = null;
        this.editingHtmlMessage = null;
        this.editingInput       = null;

        this.loadingMessages = false;
        this.reachedChatTop  = false;
        this.notifications  = false;
    }

    hideContextMenu() {
        if (this.contextMenuHtmlMessage) {
            this.contextMenuHtmlMessage.root.classList.remove("highlight");
        }

        this.contextMenuCopyUrl.classList.add("hide");
        this.contextMenuOpen.classList.add("hide");

        this.contextMenuMessage     = null;
        this.contextMenuHtmlMessage = null;
        this.contextMenuUrl         = "";
        this.contextMenuShowUrl     = false;

        hide(this.contextMenu);
    }

    showContextMenu(event, message, htmlMessage) {
        if (this.contextMenuHtmlMessage) {
            this.contextMenuHtmlMessage.root.classList.remove("highlight");
        }

        if (this.contextMenuShowUrl) {
            this.contextMenuCopyUrl.classList.remove("hide");
            this.contextMenuOpen.classList.remove("hide");
            this.contextMenuShowUrl = false;
        } else {
            this.contextMenuCopyUrl.classList.add("hide");
            this.contextMenuOpen.classList.add("hide");
        }

        if (message.user_id === this.currentUserId) {
            this.contextMenuDelete.classList.remove("hide");
            this.contextMenuEdit.classList.remove("hide");
        } else {
            this.contextMenuDelete.classList.add("hide");
            this.contextMenuEdit.classList.add("hide");
        }

        show(this.contextMenu);

        const msgRect    = htmlMessage.root.getBoundingClientRect();
        const rootRect   = this.chatListRoot.getBoundingClientRect();
        const list       = this.chatList.getBoundingClientRect();
        const menuHeight = this.contextMenu.offsetHeight;
        const width      = this.contextMenu.offsetWidth;

        let contextMenuX = event.clientX;
        let protrusion = contextMenuX + width - msgRect.right;
        if (protrusion > 0) {
            contextMenuX -= protrusion;
        }

        // Reversed columns change x,y origin of the context menu to bottom-left corner
        let revMenuBottom = list.height - event.clientY + list.top;
        let menuTop = event.clientY - menuHeight;
        if (rootRect.top > menuTop) {
            protrusion = rootRect.top - menuTop;
            revMenuBottom -= protrusion;
        }

        this.contextMenu.style.left = (contextMenuX - msgRect.left) + "px";
        this.contextMenu.style.bottom = revMenuBottom + "px";

        this.contextMenuMessage     = message;
        this.contextMenuHtmlMessage = htmlMessage;

        this.contextMenuHtmlMessage.root.classList.add("highlight");
    }

    async uploadAndPasteImage(files) {
        if (!files || files.length === 0) {
            return;
        }

        let file = files[0];
        console.log(file);
        if (!file.type.startsWith("image/")) {
            return;
        }

        let unix = Date.now();
        let filename = unix + file.name;
        let response = await api.uploadMedia(file, filename);

        let fullUrl = document.location.href + response.url;
        this.chatInput.value += fullUrl;
    }

    async loadMoreMessages() {
        if (this.loadingMessages) {
            return;
        }

        this.loadingMessages = true;
        console.info("INFO: Trying to load more messages from the server.");

        let users    = await api.userGetAll();
        let response = await api.chatGet(100, this.messages.length);
        let messages = response.json;

        if (!messages || messages.length === 0) {
            this.reachedChatTop = true;
            return;
        }

        let prevUserId = -1;
        let prevDate   = new Date();

        let htmlMessages = [];

        for (let i = 0; i < messages.length; i++) {
            let message = messages[i];
            let user    = this.findUser(message.user_id, users);
            let date    = new Date(message.created_at);

            let html;
            if (prevUserId !== user.id || !isSameDay(prevDate, date)) {
                html = this.createMessage(message, user);
            } else {
                html = this.createSubMessage(message, user);
            }

            prevUserId = user.id;
            prevDate   = date;

            htmlMessages.push(html);
        }

        for (let i = htmlMessages.length - 1; i >= 0; i--) {
            let html = htmlMessages[i];
            this.chatList.insertBefore(html.root, this.chatList.firstChild);
        }

        this.messages.unshift(...messages);
        this.htmlMessages.unshift(...htmlMessages);

        this.loadingMessages = false;
    }

    attachChatEvents() {
        this.chatListRoot.onscroll = async _ => {
            let list     = this.chatListRoot;
            let scroll   = Math.abs(list.offsetHeight - list.scrollHeight - list.scrollTop);
            let progress = (scroll / list.scrollHeight) * 100.0;

            if (progress < 10.0 && !this.reachedChatTop) {
                await this.loadMoreMessages();
            }
        };

        this.contextMenu.oncontextmenu = event => {
            event.preventDefault();
            this.hideContextMenu();
        };

        this.contextMenuCopy.onclick    = _ => navigator.clipboard.writeText(this.contextMenuMessage.content);
        this.contextMenuCopyUrl.onclick = _ => navigator.clipboard.writeText(this.contextMenuUrl);
        this.contextMenuOpen.onclick    = _ => window.open(this.contextMenuUrl, '_blank').focus();
        this.contextMenuEdit.onclick    = _ => this.startMessageEdit(this.contextMenuMessage, this.contextMenuHtmlMessage);
        this.contextMenuDelete.onclick  = _ => api.wsChatDelete(this.contextMenuMessage.id);

        this.chatList.oncontextmenu = _ => { return false };
        document.addEventListener("click", _ => this.hideContextMenu());

        this.sendMessageButton.onclick = _ => {
            this.chatInput.focus();
            this.processMessageSendIntent();
        };

        this.uploadButton.onclick = _ => {
            this.uploadImageInput.click();
        };

        this.uploadImageInput.onchange = async event => {
            let files = event.target.files;
            await this.uploadAndPasteImage(files);
        };

        // Handle shiftKey + Enter as 'new line' for formatting
        this.chatInput.onkeydown = event => {
            if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault();
                this.processMessageSendIntent();
            } else if (event.key === "Escape") {
                this.chatInput.blur();
            } else if (event.key === "ArrowUp") {
                let min = Math.min(this.messages.length, 100);

                for (let i = this.messages.length - 1; i >= this.messages.length - min; i--) {
                    const message = this.messages[i];
                    if (message.user_id === this.currentUserId) {
                        const html = this.htmlMessages[i];
                        this.chatInput.scrollTo(html);
                        this.startMessageEdit(message, html);
                        break;
                    }
                }
            }
        };

        this.contentRoot.ondragover = event => {
            event.preventDefault();
        };

        this.contentRoot.ondrop = event => {
            event.preventDefault();

            let data = event.dataTransfer;
            this.uploadAndPasteImage(data.files);
        };

        this.chatInput.onpaste = event => {
            let data = event.clipboardData;
            this.uploadAndPasteImage(data.files);
        };
    }

    clear() {
        this.messages     = [];
        this.htmlMessages = [];

        this.prevUserId = -1;
        this.prevDate   = new Date(); 

        while (this.chatList.lastChild) {
            this.chatList.removeChild(this.chatList.lastChild);
        }

        this.hideContextMenu();

        this.editingMessage     = null;
        this.editingHtmlMessage = null;
        this.editingInput       = null;
    }

    findUser(userId, allUsers) {
        let user = allUsers.find(user => user.id === userId);

        if (!user) {
            const dummy = {
                id: 0,
                username: "Deleted user",
                avatar:   "img/default_avatar.png",
                online:   false,
            };

            user = dummy;
        }

        return user
    }

    createMessage(message, user) {
        let root      = div("chat_message");
        let avatar    = div("chat_message_avatar");
        let avatarImg = img(user.avatar);
        let right     = div("chat_message_right");
        let info      = div("chat_message_info");
        let username  = div("chat_message_username");
        let date      = div("chat_message_date");
        let text      = div("chat_message_text");

        let segments = this.linkify(message.content);
        username.textContent = user.username;

        if (user.id !== 0) {
            let color = Math.floor(Math.sin(user.id) * 10000);
            username.style.color = `hsl(${color} 70% 50%)`
        } 

        let createdDate = new Date(message.created_at);
        let [Y, M, D, h, m] = getDateStrings(createdDate);

        let now = new Date();
        if (isSameDay(createdDate, now)) {
            date.textContent = `${h}:${m}`;
        } else {
            date.textContent = `${Y}/${M}/${D}, ${h}:${m}`;
        }

        let html = {
            root: root,
            text: text,
        };

        root.oncontextmenu = event => {
            event.preventDefault();

            if (this.contextMenuMessage && this.contextMenuMessage.id === message.id) {
                this.hideContextMenu();
            } else {
                this.showContextMenu(event, message, html);
            }
        };

        root.appendChild(avatar); {
            avatar.appendChild(avatarImg);
        }
        root.appendChild(right); {
            right.appendChild(info); {
                info.appendChild(username);
                info.appendChild(date);
            }
            right.appendChild(text); {
                text.append(...segments);
            }
        }

        return html
    }

    createSubMessage(message) {
        let root = div("chat_sub_message");
        let date = div("chat_sub_message_date");
        let text = div("chat_sub_message_text");

        let segments = this.linkify(message.content);

        let d = new Date(message.created_at);
        let h = d.getHours().toString().padStart(2, "0");
        let m = d.getMinutes().toString().padStart(2, "0");

        date.textContent = `${h}:${m}`;

        let html = {
            root: root,
            text: text,
        };

        root.oncontextmenu = event => {
            event.preventDefault();

            if (this.contextMenuMessage && this.contextMenuMessage.id === message.id) {
                this.hideContextMenu();
            } else {
                this.showContextMenu(event, message, html);
            }
        };

        root.appendChild(date);
        root.appendChild(text); {
            text.append(...segments);
        }

        return html
    }

    contextUrlShow(url) {
        this.contextMenuUrl     = url;
        this.contextMenuShowUrl = true;
    }

    contextUrlHide() {
        this.contextMenuUrl     = "";
        this.contextMenuShowUrl = false;
    }

    linkify(content) {
        let segments = [];

        let segmentStart = 0;
        let parsingUrl   = false;

        for (let i = 0; i < content.length; i++) {
            let slice = content.slice(i);

            if (!parsingUrl && (slice.startsWith("http://") || slice.startsWith("https://"))) {
                if (segmentStart !== i) {
                    let text    = content.slice(segmentStart, i);
                    let segment = span(null, text);
                    segment.oncontextmenu = _ => this.contextUrlHide();
                    segments.push(segment);
                }

                segmentStart = i;
                parsingUrl   = true;
            }

            let rune = content[i];
            if (rune === " " && parsingUrl) {
                let url = content.slice(segmentStart, i);

                let segment;
                if (isLocalImage(url)) {
                    segment = img(url);
                } else {
                    segment = a(null, url)
                }

                segment.oncontextmenu = _ => this.contextUrlShow(url);
                segments.push(segment);

                parsingUrl   = false;
                segmentStart = i;
            }
        }

        if (parsingUrl) {
            let url = content.slice(segmentStart);

            let segment;
            if (isLocalImage(url)) {
                segment = img(url);
            } else {
                segment = a(null, url)
            }

            segment.oncontextmenu = _ => this.contextUrlShow(url);
            segments.push(segment);
        } else {
            let text    = content.slice(segmentStart);
            let segment = span(null, text);
            segment.oncontextmenu = _ => this.contextUrlHide();
            segments.push(segment);
        }

        return segments
    }

    addMessage(chatMsg, allUsers, isNew = false) {
        let user = this.findUser(chatMsg.user_id, allUsers);
        let date = new Date(chatMsg.created_at);

        if (this.notifications && isNew && this.currentUserId !== chatMsg.user_id) {
            new Notification(user.username, {
                body: chatMsg.content,
            });
        }

        let message;
        if (this.prevUserId !== user.id || !isSameDay(this.prevDate, date)) {
            message = this.createMessage(chatMsg, user);
        } else {
            message = this.createSubMessage(chatMsg, user);
        }

        this.prevUserId = user.id;
        this.prevDate   = date;

        this.messages.push(chatMsg);
        this.htmlMessages.push(message);
        this.chatList.appendChild(message.root);
    }

    createEditInputBox(messageContent) {
        let root       = div("chat_edit_container");
        let inputBox   = input("chat_edit_input", messageContent, "Edit your message.");
        let sendButton = button("chat_edit_send_button", "Send message.");
        let sendSvg    = svg("svg/main_icons.svg#send");

        let html = {
            root:  root,
            input: inputBox,
        };

        inputBox.onkeydown = event => {
            if (event.key === "Enter") {
                this.stopMessageEdit();
            } else if (event.key === "Escape") {
                this.cancelMessageEdit();
            }
        };

        sendButton.onclick = _ => {
            this.stopMessageEdit();
        };

        root.appendChild(inputBox);
        root.appendChild(sendButton); {
            sendButton.appendChild(sendSvg);
        }

        return html;
    }

    startMessageEdit(message, htmlMessage) {
        if (message.user_id !== this.currentUserId) {
            console.warn("WARN: User ID", this.currentUserId, "is not allowed to edit message:", message);
            return;
        }

        if (this.editingMessage) {
            if (this.editingMessage.id === message.id) {
                this.editingInput.focus();
                return;
            } else {
                this.cancelMessageEdit();
            }
        }

        let editHtml = this.createEditInputBox(message.content);
        let inputBox = editHtml.input;
        
        clearContent(htmlMessage.text);
        htmlMessage.text.appendChild(editHtml.root);

        inputBox.focus();
        // HACK: 
        //     Input box won't set the cursor position immediately after it is appended to DOM (because web reasons),
        //     so we need to wait an [[[arbitrary]]] amount of time, for it become interactive.
        setTimeout(_ => inputBox.setSelectionRange(inputBox.value.length, inputBox.value.length), 16);

        this.editingMessage     = message;
        this.editingHtmlMessage = htmlMessage;
        this.editingInput       = editHtml.input;
    }

    cancelMessageEdit() {
        clearContent(this.editingHtmlMessage.text);

        let segments = this.linkify(this.editingMessage.content);
        this.editingHtmlMessage.text.append(...segments);

        this.editingMessage     = null;
        this.editingHtmlMessage = null;
        this.editingInput       = null;

        this.chatInput.focus();
    }

    stopMessageEdit() {
        let content = this.editingInput.value.trim();
        if (content === "") {
            this.cancelMessageEdit();
            return;
        }

        clearContent(this.editingHtmlMessage.text);
        api.wsChatEdit(this.editingMessage.id, content);

        this.editingMessage     = null;
        this.editingHtmlMessage = null;
        this.editingInput       = null;

        this.chatInput.focus();
    }

    edit(messageId, messageContent) {
        let index = this.messages.findIndex(message => message.id === messageId);
        if (index === -1) {
            console.warn("WARN: Chat::edit failed. Failed to find message with ID =", messageId);
            return;
        }

        let message = this.messages[index];
        let html    = this.htmlMessages[index];

        message.content = messageContent;

        if (this.editingMessage && this.editingMessage.id === message.id) {
            return;
        }

        clearContent(html.text);
        let segments = this.linkify(messageContent);
        html.text.append(...segments);
    }

    delete(messageId, allUsers) {
        let index = this.messages.findIndex(message => message.id === messageId);
        if (index === -1) {
            console.warn("WARN: Chat::delete failed. Failed to find message with ID =", messageId);
            return;
        }

        let message = this.messages[index];
        let html    = this.htmlMessages[index];

        if (index === this.messages.length - 1) {
            let prev = this.messages[index - 1];

            if (prev) {
                this.prevUserId = prev.user_id;
                this.prevDate   = new Date(prev.created_at);
            } else {
                this.prevUserId = -1;
                this.prevDate   = new Date();
            }
        }

        if (html.root.classList.contains("chat_message")) {
            let next     = this.messages[index + 1];
            let nextHtml = this.htmlMessages[index + 1];
            if (next && !nextHtml.root.classList.contains("chat_message")) {
                let user = this.findUser(next.user_id, allUsers);
                let newHtml = this.createMessage(next, user);

                if (this.contextMenuMessage && this.contextMenuMessage.id === next.id) {
                    this.contextMenuHtmlMessage = newHtml;
                    newHtml.root.classList.add("highlight");
                }

                this.htmlMessages[index + 1] = newHtml;
                this.chatList.replaceChild(newHtml.root, nextHtml.root);
            }
        }

        if (this.contextMenuMessage && this.contextMenuMessage.id === message.id) {
            this.hideContextMenu();
        }

        if (this.editingMessage && this.editingMessage.id === message.id) {
            this.editingMessage     = null;
            this.editingHtmlMessage = null;
            this.editingInput       = null;
        }

        this.messages.splice(index, 1);
        this.htmlMessages.splice(index, 1);
        this.chatList.removeChild(html.root);
    }

    processMessageSendIntent() {
        let content = this.chatInput.value.trim();
        this.chatInput.value = "";

        if (content.length === 0 || content.length > CHARACTER_LIMIT) {
            console.warn("WARN: Message is empty or exceeds", CHARACTER_LIMIT, "characters");
            // This is handled server side for length
            return;
        }

        api.wsChatSend(content);
    }

    loadMessages(messages, allUsers) {
        for (let i = 0; i < messages.length; i++) {
            this.addMessage(messages[i], allUsers);
        }
    }
} 
