package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func (server *Server) handleEndpointAuthorized(mux *http.ServeMux, endpoint string, endpointHandler func(w http.ResponseWriter, r *http.Request, userId uint64), method string) {
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				LogFatalUp(2, "Panic in endpoint handler for %v serving %v: %v", endpoint, getIp(r), err)
			}
		}()

		if r.Method != method {
			errMsg := fmt.Sprintf("Method not allowed. %v was expected.", method)
			http.Error(w, errMsg, http.StatusMethodNotAllowed)
			return
		}

		user := server.getAuthorized(w, r)
		if user == nil {
			return
		}

		endpointTrim := strings.TrimPrefix(endpoint, "/api/")
		requested := strings.ReplaceAll(endpointTrim, "/", " ")
		LogInfo("Connection %s requested %v.", getIp(r), requested)

		endpointHandler(w, r, user.Id)
	}

	mux.HandleFunc(endpoint, genericHandler)
}

func (server *Server) handleEndpoint(mux *http.ServeMux, endpoint string, endpointHandler func(w http.ResponseWriter, r *http.Request), method string) {
	genericHandler := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				LogFatalUp(2, "Panic in endpoint handler for %v serving %v: %v", endpoint, getIp(r), err)
			}
		}()

		if r.Method != method {
			errMsg := fmt.Sprintf("Method not allowed. %v was expected.", method)
			http.Error(w, errMsg, http.StatusMethodNotAllowed)
			return
		}

		// NOTE(kihau): Hack to prevent console spam on proxy.
		if PROXY_ROUTE != endpoint && STREAM_ROUTE != endpoint {
			endpointTrim := strings.TrimPrefix(endpoint, "/api/")
			requested := strings.ReplaceAll(endpointTrim, "/", " ")
			LogInfo("Connection %s requested %v.", getIp(r), requested)
		}

		endpointHandler(w, r)
	}

	mux.HandleFunc(endpoint, genericHandler)
}

func (server *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	buildTime := fmt.Sprintf("%v_%v", VERSION, BuildTime)
	response, _ := json.Marshal(buildTime)
	w.Write(response)
}

func (server *Server) apiUptime(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(startTime)
	uptimeString := fmt.Sprintf("%v", uptime)
	response, _ := json.Marshal(uptimeString)
	w.Write(response)
}

func (server *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s attempted to log in.", getIp(r))
	io.WriteString(w, "This is unimplemented")
}

func (server *Server) apiUploadMedia(w http.ResponseWriter, r *http.Request, userId uint64) {
	inputFile, headers, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "Failed to read formdata from the request: %v", err)
		return
	}

	filename := SanitizeUrlFileName(headers.Filename)
	// TODO(kihau): Actually check file format by doing http.DetectContentType().
	extension := path.Ext(filename)
	directory := getMediaType(extension)

	outputPath, isSafe := safeJoin(CONTENT_MEDIA, directory, filename)
	if !isSafe {
		respondBadRequest(w, "Filename of the uploaded file is not allowed")
		return
	}
	os.MkdirAll(CONTENT_MEDIA+directory, os.ModePerm)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		respondInternalError(w, "Server side file creation for file %v failed with: %v", outputPath, err)
		return
	}
	defer outputFile.Close()

	LogInfo("Saving uploaded media file to: %v.", outputPath)

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		respondInternalError(w, "Server side file copy for file %v failed with: %v", outputPath, err)
		return
	}

	networkPath, isSafe := safeJoin(CONTENT_MEDIA, directory, filename)
	if !isSafe {
		respondBadRequest(w, "Filename of the uploaded file is not allowed")
		return
	}

	trimmed := strings.TrimSuffix(filename, extension)
	name := cleanupResourceName(trimmed)

	response := MediaUploadResponse{
		Url:      networkPath,
		Name:     name,
		Filename: filename,
		Format:   extension,
		Category: directory,
	}

	jsonData, _ := json.Marshal(response)
	w.Write(jsonData)
}

func (server *Server) apiInviteCreate(w http.ResponseWriter, r *http.Request, userId uint64) {
	invite := server.createNewInvite(userId)
	jsonData, _ := json.Marshal(invite)
	w.Write(jsonData)
}

func (server *Server) apiShare(w http.ResponseWriter, r *http.Request, userId uint64) {
	var request ShareResourceRequest
	if !server.readJsonDataFromRequest(w, r, &request) {
		return
	}

	if request.LifetimeSeconds > MAX_SHARE_LIFETIME_SECONDS {
		request.LifetimeSeconds = MAX_SHARE_LIFETIME_SECONDS
	}
	parsedUrl, err := url.Parse(request.Url)
	if err != nil {
		respondBadRequest(w, "Failed to parse url: %v", err)
		return
	}
	safePath, safe := safeJoin(parsedUrl.Path)
	if !safe {
		respondBadRequest(w, "Invalid path.")
		return
	}
	if !fileExists(safePath) {
		respondBadRequest(w, "File does not exist.")
		return
	}
	expiresAt := time.Now().Add(time.Duration(request.LifetimeSeconds) * time.Second)
	key := randomBase64(10)
	relativeSharedPath := "/share/" + key
	server.state.resourceLock.Lock()
	server.state.resources[key] = SharedResource{
		path:    safePath,
		expires: expiresAt,
	}
	server.state.resourceLock.Unlock()
	response := ShareResourceResponse{SharedPath: relativeSharedPath}
	jsonResponse, _ := json.Marshal(response)
	w.Write(jsonResponse)
}

func (server *Server) apiUserCreate(w http.ResponseWriter, r *http.Request) {
	var inviteCode string
	if !server.readJsonDataFromRequest(w, r, &inviteCode) {
		return
	}

	server.users.mutex.Lock()
	userCount := len(server.users.slice)
	server.users.mutex.Unlock()

	if userCount != 0 {
		server.state.mutex.Lock()
		invite := server.state.invite

		if invite.InviteCode != inviteCode {
			server.state.mutex.Unlock()
			respondBadRequest(w, "Provided invite code is not valid.")
			return
		}

		if invite.ExpiresAt.Before(time.Now()) {
			server.state.mutex.Unlock()
			respondBadRequest(w, "Provided invite code has expired.")
			return
		}

		server.state.invite = Invite{}
		server.state.mutex.Unlock()
	}

	user, err := server.createUser()
	if err != nil {
		respondInternalError(w, "Failed to create user. Database error occurred: %v", err)
		return
	}

	tokenJson, err := json.Marshal(user.token)
	if err != nil {
		respondInternalError(w, "Serialization of the user token failed with: %v", err)
		return
	}

	server.writeEventToAllConnections("usercreate", user, SERVER_ID)
	w.Write(tokenJson)
}

func (server *Server) apiUserVerify(w http.ResponseWriter, r *http.Request) {
	var token string
	if !server.readJsonDataFromRequest(w, r, &token) {
		return
	}

	server.users.mutex.Lock()
	user := server.findUser(token)
	server.users.mutex.Unlock()
	if user == nil {
		respondBadRequest(w, "User with specified token was not found")
		return
	}

	response := UserVerifyResponse{
		UserId:   user.Id,
		PagePath: PAGE_ROOT,
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		respondInternalError(w, "Serialization of the user id failed with: %v", err)
		return
	}

	w.Write(jsonData)
}

func (server *Server) apiUserDelete(w http.ResponseWriter, r *http.Request) {
	var token string
	if !server.readJsonDataFromRequest(w, r, &token) {
		return
	}

	server.users.mutex.Lock()
	user := server.users.removeByToken(token)
	server.users.mutex.Unlock()

	if user == nil {
		respondBadRequest(w, "User with specified token not found")
		return
	}

	DatabaseDeleteUser(server.db, *user)
	server.writeEventToAllConnections("userdelete", user, SERVER_ID)

	server.conns.mutex.Lock()
	for _, conn := range server.conns.slice {
		if conn.userId == user.Id {
			select {
			case conn.close <- true:
			default:
			}
		}
	}
	server.conns.mutex.Unlock()
}

func (server *Server) apiUserGetAll(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.users.mutex.Lock()
	usersJson, err := json.Marshal(server.users.slice)
	server.users.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the users list failed with: %v", err)
		return
	}

	w.Write(usersJson)
}

func (server *Server) apiUserUpdateName(w http.ResponseWriter, r *http.Request, userId uint64) {
	var newUsername string
	if !server.readJsonDataFromRequest(w, r, &newUsername) {
		return
	}

	if len(newUsername) > MAX_NICKNAME_LENGTH {
		respondBadRequest(w, "Nickname is too long, exceeds %v bytes", MAX_NICKNAME_LENGTH)
		return
	}
	if !validateName(newUsername) {
		respondBadRequest(w, "Nickname is invalid due to one of the following reasons: the nickname is empty, contains invalid separators, contains invalid UTF8, contains CDM characters")
		return
	}
	newUsername = strings.TrimSpace(newUsername)

	server.users.mutex.Lock()
	userIndex := server.getAuthorizedIndex(w, r)

	if userIndex == -1 {
		server.users.mutex.Unlock()
		return
	}

	server.users.slice[userIndex].Username = newUsername
	server.users.slice[userIndex].LastUpdate = time.Now()
	user := server.users.slice[userIndex]
	DatabaseUpdateUser(server.db, user)
	server.users.mutex.Unlock()

	server.writeEventToAllConnections("userupdate", user, userId)
}

func (server *Server) apiUserUpdateAvatar(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.users.mutex.Lock()
	userIndex := server.getAuthorizedIndex(w, r)
	if userIndex == -1 {
		server.users.mutex.Unlock()
		return
	}
	user := server.users.slice[userIndex]
	server.users.mutex.Unlock()

	if r.ContentLength > AVATAR_SIZE_LIMIT {
		http.Error(w, "The avatar is too large in size.", http.StatusRequestEntityTooLarge)
		LogWarn("User wanted to upload an avatar %v bytes in size", r.ContentLength)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, AVATAR_SIZE_LIMIT)
	formfile, _, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "Failed to read form data from the user avatar change request: %v", err)
		return
	}

	var fileContents [512]byte
	n, err := formfile.Read(fileContents[:])
	if err != nil && err != io.EOF {
		respondInternalError(w, "Error reading from disk")
		return
	}

	contentType := http.DetectContentType(fileContents[:n])
	var ext string
	switch contentType {
	case "image/png":
		ext = "png"
	case "image/jpeg":
		ext = "jpg"
	case "image/gif":
		ext = "gif"
	case "image/webp":
		ext = "webp"
	default:
		respondBadRequest(w, "Invalid image type, detected: %v", contentType)
		return
	}

	os.MkdirAll(CONTENT_USERS, os.ModePerm)
	avatarUrl := fmt.Sprintf(CONTENT_USERS+"avatar%v", user.Id)

	os.Remove(avatarUrl)
	file, err := os.Create(avatarUrl)
	if err != nil {
		respondInternalError(w, "File creation for the user avatar file failed with: %v", err)
		return
	}
	defer file.Close()

	file.Write(fileContents[:n])
	io.Copy(file, formfile)

	// Unix timestamp is added because of HTML DOM URL caching.
	now := time.Now()
	avatarUrl = fmt.Sprintf(CONTENT_USERS+"avatar%v?ext=%v&%v", user.Id, ext, now.UnixMilli())

	server.users.mutex.Lock()
	server.users.slice[userIndex].Avatar = avatarUrl
	server.users.slice[userIndex].LastUpdate = time.Now()
	user = server.users.slice[userIndex]
	DatabaseUpdateUser(server.db, user)
	server.users.mutex.Unlock()

	jsonData, _ := json.Marshal(avatarUrl)

	server.writeEventToAllConnections("userupdate", user, userId)
	w.Write(jsonData)
}

func (server *Server) apiPlayerGet(w http.ResponseWriter, r *http.Request, userId uint64) {
	getEvent := server.playerGet()

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		respondInternalError(w, "Serialization of the get event failed with: %v", err)
		return
	}

	w.Write(jsonData)
}

func (server *Server) apiPlayerSet(w http.ResponseWriter, r *http.Request, userId uint64) {
	var requested RequestEntry
	if !server.readJsonDataFromRequest(w, r, &requested) {
		return
	}

	server.playerSet(requested, userId)
}

func (server *Server) apiPlayerNext(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data PlayerNextRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	if err := server.playerNext(data, userId); err != nil {
		respondBadRequest(w, "%v", err)
	}
}

func (server *Server) apiPlayerPlay(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data PlayerSyncRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.playerPlay(data, userId)
}

func (server *Server) apiPlayerPause(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data PlayerSyncRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.playerPause(data, userId)
}

func (server *Server) apiPlayerSeek(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data PlayerSyncRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.playerSeek(data, userId)
}

func (server *Server) apiPlayerAutoplay(w http.ResponseWriter, r *http.Request, userId uint64) {
	var autoplay bool
	if !server.readJsonDataFromRequest(w, r, &autoplay) {
		return
	}

	server.playerAutoplay(autoplay, userId)
}

func (server *Server) apiPlayerLooping(w http.ResponseWriter, r *http.Request, userId uint64) {
	var looping bool
	if !server.readJsonDataFromRequest(w, r, &looping) {
		return
	}

	server.playerLooping(looping, userId)
}

func (server *Server) apiPlayerUpdateTitle(w http.ResponseWriter, r *http.Request, userId uint64) {
	var title string
	if !server.readJsonDataFromRequest(w, r, &title) {
		return
	}

	server.playerUpdateTitle(title, userId)
}

func (server *Server) apiSubtitleFetch(w http.ResponseWriter, r *http.Request, userId uint64) {
	if !server.config.EnableSubs {
		// Subs is a different feature...
		http.Error(w, "Feature unavailable", http.StatusServiceUnavailable)
		return
	}

	err := server.fetchLyricsForCurrentEntry(userId)
	if err != nil {
		respondInternalError(w, err.Error())
	} else {
		io.WriteString(w, "{}")
	}
}

func (server *Server) apiSubtitleDelete(w http.ResponseWriter, r *http.Request, userId uint64) {
	var subId uint64
	if !server.readJsonDataFromRequest(w, r, &subId) {
		return
	}

	server.state.mutex.Lock()
	for i, sub := range server.state.entry.Subtitles {
		if sub.Id == subId {
			subs := server.state.entry.Subtitles
			server.state.entry.Subtitles = slices.Delete(subs, i, i+1)
			DatabaseSubtitleDelete(server.db, sub.Id)
			break
		}
	}
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("subtitledelete", subId, userId)
}

func (server *Server) apiSubtitleUpdate(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data SubtitleUpdateRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	for i, sub := range server.state.entry.Subtitles {
		if sub.Id == data.Id {
			server.state.entry.Subtitles[i].Name = data.Name
			break
		}
	}

	DatabaseSubtitleUpdate(server.db, data.Id, data.Name)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("subtitleupdate", data, userId)
}

func (server *Server) apiSubtitleAttach(w http.ResponseWriter, r *http.Request, userId uint64) {
	var subtitle Subtitle
	if !server.readJsonDataFromRequest(w, r, &subtitle) {
		return
	}

	// TODO(kihau): Validate subtitle URL (but preferably change the entires subtitle upload/download/attach system)

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if err := DatabaseSubtitleAdd(server.db, server.state.entry.Id, &subtitle); err != nil {
		respondInternalError(w, "Failed to attach a subtitle, looks like that database is broken: %v", err)
		return
	}
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.addRecentAction("subtitleattach", userId, subtitle)

	server.writeEventToAllConnections("subtitleattach", subtitle, userId)
}

func (server *Server) apiSubtitleShift(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data SubtitleShiftRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	for i, sub := range server.state.entry.Subtitles {
		if sub.Id == data.Id {
			server.state.entry.Subtitles[i].Shift = data.Shift
			break
		}
	}

	DatabaseSubtitleShift(server.db, data.Id, data.Shift)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("subtitleshift", data, userId)
}

func (server *Server) apiSubtitleUpload(w http.ResponseWriter, r *http.Request, userId uint64) {
	// Ensure that file content doesn't exceed maximum subtitle size limit.
	r.Body = http.MaxBytesReader(w, r.Body, SUBTITLE_SIZE_LIMIT)

	inputFile, headers, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "Failed to read form data from the subtitle upload request: %v", err)
		return
	}

	if headers.Size > SUBTITLE_SIZE_LIMIT {
		http.Error(w, "Subtitle file is too large", http.StatusRequestEntityTooLarge)
		return
	}

	subtitle := createSubtitle(headers.Filename, path.Ext(headers.Filename))
	outputFile, err := os.Create(subtitle.Url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer outputFile.Close()

	LogInfo("Saving uploaded subtitle file to: %v.", subtitle.Url)

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		respondInternalError(w, "Failed to save the subtitle file file: %v", err)
		return
	}

	jsonData, _ := json.Marshal(subtitle)
	w.Write(jsonData)
}

func (server *Server) apiSubtitleDownload(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data SubtitleDownloadRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	url, err := url.Parse(data.Url)
	if err != nil {
		respondBadRequest(w, "Provided url '%v' is invalid: %v", data.Url, err)
		return
	}

	// Cut at fragment identifier if present
	hash := strings.Index(data.Url, "#")
	if hash >= 0 {
		data.Url = data.Url[:hash]
	}
	filename := filepath.Base(data.Url)
	extension := path.Ext(filename)
	serverUrl := data.Url

	if url.IsAbs() {
		downloadOptions := DownloadOptions{
			hasty:     true,
			referer:   data.Referer,
			bodyLimit: SUBTITLE_SIZE_LIMIT,
		}
		os.MkdirAll(CONTENT_SUBS, os.ModePerm)

		randomString := generateToken()
		outputName := fmt.Sprintf("%v%v", randomString, extension)
		serverUrl = path.Join(CONTENT_SUBS, outputName)
		outputPath := path.Join(CONTENT_SUBS, outputName)

		err = downloadFile(data.Url, outputPath, &downloadOptions)
		if err != nil {
			var downloadErr *DownloadError
			if errors.As(err, &downloadErr) {
				respondBadRequest(w, "Failed to download subtitle from '%v', status %v", data.Url, downloadErr.Code)
			} else {
				respondBadRequest(w, "Failed to download subtitle from '%v', due to %v", data.Url, err)
			}
		}
	}

	name := data.Name
	if name == "" {
		name = strings.TrimSuffix(filename, extension)
	}
	name = cleanupResourceName(name)

	subtitle := Subtitle{
		Name:  name,
		Url:   serverUrl,
		Shift: 0.0,
	}

	jsonData, _ := json.Marshal(subtitle)
	w.Write(jsonData)
}

func (server *Server) apiSubtitleSearch(w http.ResponseWriter, r *http.Request, userId uint64) {
	if !server.config.EnableSubs {
		http.Error(w, "Feature unavailable", http.StatusServiceUnavailable)
		return
	}

	var search Search
	if !server.readJsonDataFromRequest(w, r, &search) {
		return
	}

	os.MkdirAll(MEDIA_SUBS, os.ModePerm)
	downloadPath, err := downloadSubtitle(&search, MEDIA_SUBS)
	if err != nil {
		respondBadRequest(w, "Subtitle download failed: %v", err)
		return
	}

	inputSub, err := os.Open(downloadPath)
	if err != nil {
		respondInternalError(w, "Failed to open downloaded subtitle %v: %v", downloadPath, err)
		return
	}
	defer inputSub.Close()

	extension := path.Ext(downloadPath)
	baseName := filepath.Base(downloadPath)
	subtitle := createSubtitle(baseName, extension)

	outputSub, err := os.Create(subtitle.Url)
	if err != nil {
		respondInternalError(w, "Failed to created output subtitle in %v: %v", subtitle.Url, err)
		return
	}

	defer outputSub.Close()

	_, err = io.Copy(outputSub, inputSub)
	if err != nil {
		respondInternalError(w, "Failed to copy downloaded subtitle file: %v", err)
		return
	}

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if err := DatabaseSubtitleAdd(server.db, server.state.entry.Id, &subtitle); err != nil {
		respondInternalError(w, "Subtite search failed, looks like that database is broken: %v", err)
		return
	}
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.writeEventToAllConnections("subtitleattach", subtitle, userId)
}

func (server *Server) apiPlaylistGet(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.playlist)
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the playlist get event failed with: %v", err)
		return
	}

	w.Write(jsonData)
}

func (server *Server) apiPlaylistPlay(w http.ResponseWriter, r *http.Request, userId uint64) {
	var entryId uint64
	if !server.readJsonDataFromRequest(w, r, &entryId) {
		return
	}

	if err := server.playlistPlay(entryId, userId); err != nil {
		respondBadRequest(w, "%v", err)
	}
}

func (server *Server) apiPlaylistAdd(w http.ResponseWriter, r *http.Request, userId uint64) {
	var requested RequestEntry
	if !server.readJsonDataFromRequest(w, r, &requested) {
		return
	}

	server.playlistAdd(requested, userId)
}

func (server *Server) apiPlaylistClear(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.playlistClear()
}

func (server *Server) apiPlaylistDelete(w http.ResponseWriter, r *http.Request, userId uint64) {
	var entryId uint64
	if !server.readJsonDataFromRequest(w, r, &entryId) {
		return
	}

	if err := server.playlistDelete(entryId, userId); err != nil {
		respondBadRequest(w, "%v", err)
	}
}

func (server *Server) apiPlaylistShuffle(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.playlistShuffle()
}

func (server *Server) apiPlaylistMove(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data PlaylistMoveRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	if err := server.playlistMove(data, userId); err != nil {
		respondBadRequest(w, "%v", err)
	}
}

func (server *Server) apiPlaylistUpdate(w http.ResponseWriter, r *http.Request, userId uint64) {
	var entry Entry
	if !server.readJsonDataFromRequest(w, r, &entry) {
		return
	}

	if err := server.playlistUpdate(entry, userId); err != nil {
		respondBadRequest(w, "%v", err)
	}
}

func (server *Server) apiHistoryGet(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.history)
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the history get event failed with: %v", err)
		return
	}

	w.Write(jsonData)
}

func (server *Server) apiChatGet(w http.ResponseWriter, r *http.Request, userId uint64) {
	var data MessageHistoryRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	messages, err := server.chatGet(data, userId)
	if err != nil {
		server.state.mutex.Unlock()
		respondBadRequest(w, err.Error())
	}

	jsonData, err := json.Marshal(messages)
	server.state.mutex.Unlock()

	if err != nil {
		respondBadRequest(w, "Serialization failed during chat retrieval: %v", err)
		return
	}

	w.Write(jsonData)
}

func indexOfMessageById(messages []ChatMessage, messageId uint64) int {
	for b := len(messages) - 1; b >= 0; b-- {
		if messages[b].Id == messageId {
			return b
		}
	}
	return -1
}

func (server *Server) apiChatDelete(w http.ResponseWriter, r *http.Request, userId uint64) {
	var messageId uint64
	if !server.readJsonDataFromRequest(w, r, &messageId) {
		return
	}

	server.chatDelete(messageId, userId)
}

func (server *Server) apiHistoryClear(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.state.mutex.Lock()
	server.state.history = server.state.history[:0]
	server.state.mutex.Unlock()

	DatabaseHistoryClear(server.db)

	server.writeEventToAllConnections("historyclear", nil, userId)
}

func (server *Server) apiHistoryPlay(w http.ResponseWriter, r *http.Request, userId uint64) {
	var entryId uint64
	if !server.readJsonDataFromRequest(w, r, &entryId) {
		return
	}

	server.state.mutex.Lock()
	index := FindEntryIndex(server.state.history, entryId)
	if index == -1 {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Failed to play history element. Entry with ID %v is not in the history.", entryId)
		return
	}

	entry := server.state.history[index]
	server.state.mutex.Unlock()

	server.setNewEntry(entry, userId)
}

func (server *Server) apiHistoryDelete(w http.ResponseWriter, r *http.Request, userId uint64) {
	var entryId uint64
	if !server.readJsonDataFromRequest(w, r, &entryId) {
		return
	}

	server.state.mutex.Lock()
	index := FindEntryIndex(server.state.history, entryId)
	if index == -1 {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Failed to remove history element. Entry with ID %v is not in the history.", entryId)
		return
	}

	server.historyDelete(index)
	server.state.mutex.Unlock()
}

func (server *Server) apiHistoryPlaylistAdd(w http.ResponseWriter, r *http.Request, userId uint64) {
	var entryId uint64
	if !server.readJsonDataFromRequest(w, r, &entryId) {
		return
	}

	if err := server.historyPlaylistAdd(entryId); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
	}
}

func (server *Server) apiChatSend(w http.ResponseWriter, r *http.Request, userId uint64) {
	var messageContent string
	if !server.readJsonDataFromRequest(w, r, &messageContent) {
		return
	}

	if err := server.chatCreate(messageContent, userId); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
	}
}

func (server *Server) apiChatEdit(w http.ResponseWriter, r *http.Request, userId uint64) {
	var messageEdit ChatMessageEdit
	if !server.readJsonDataFromRequest(w, r, &messageEdit) {
		return
	}

	if err := server.chatEdit(messageEdit, userId); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
	}
}

func (server *Server) apiStreamStart(w http.ResponseWriter, r *http.Request, userId uint64) {
	LogInfo("Connection %s started stream.", getIp(r))

	server.state.setupLock.Lock()
	defer server.state.setupLock.Unlock()

	_ = os.RemoveAll(CONTENT_STREAM)
	_ = os.MkdirAll(CONTENT_STREAM, os.ModePerm)

	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	entryUrl := STREAM_ROUTE + "stream.m3u8"
	m3u := M3U{
		url:    entryUrl,
		isLive: true,
	}
	m3u.addPair(KeyValue{EXT_X_VERSION, "3"})
	m3u.addPair(KeyValue{EXT_X_TARGETDURATION, "10"})
	m3u.addPair(KeyValue{EXT_X_MEDIA_SEQUENCE, "0"})

	// Create a dummy file so clients have something to munch on
	file, err := os.Create(CONTENT_STREAM + STREAM_M3U8)
	if err != nil {
		respondInternalError(w, "ERROR: %v", err)
		return
	}

	defer file.Close()
	m3u.serializePlaylist(file)

	server.state.liveStream = LiveStream{}

	entry := Entry{
		Url:       entryUrl,
		UserId:    user.Id,
		Title:     user.Username + "'s stream",
		UseProxy:  false,
		Subtitles: []Subtitle{},
		CreatedAt: time.Now(),
	}

	go server.setNewEntry(entry, userId)
}

func (server *Server) apiStreamUpload(w http.ResponseWriter, r *http.Request, userId uint64) {
	server.state.mutex.Lock()
	entryUserId := server.state.entry.UserId
	server.state.mutex.Unlock()

	if entryUserId != userId {
		LogWarn("User ID mismatch on stream upload from %v", getIp(r))
		http.Error(w, "You're not the owner of this stream", http.StatusUnauthorized)
		return
	}

	filename := r.PathValue("filename")
	LogInfo("Receiving stream item: %v", filename)

	// Do more filename validation (continuity)
	if !strings.HasPrefix(filename, "stream") {
		respondBadRequest(w, "Invalid filename %v", filename)
		return
	}

	safePath, safe := safeJoin(CONTENT_STREAM, filename)
	if !safe {
		respondBadRequest(w, "Path traversal attempted on %v", filename)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MAX_STREAM_CHUNK_SIZE)

	server.state.setupLock.Lock()
	liveStream := &server.state.liveStream
	server.state.setupLock.Unlock()

	file, err := os.Create(safePath)
	if err != nil {
		LogError("Failed to create file: %v", err)
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	written, err := io.Copy(file, r.Body)
	if err != nil {
		LogError("Failed to read bytes from body: %v", err)
		http.Error(w, "Failed to read bytes", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()
	liveStream.dataTransferred += written

	w.WriteHeader(http.StatusOK)
}

func (server *Server) apiEvents(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		respondBadRequest(w, "Failed to connect to event stream. User token missing")
		return
	}

	server.users.mutex.Lock()
	user := server.findUser(token)
	if user == nil {
		respondBadRequest(w, "Failed to connect to event stream. User with specified token not found.")
		server.users.mutex.Unlock()
		return
	}

	ws, err := server.conns.upgrader.Upgrade(w, r, nil)
	if err != nil {
		respondBadRequest(w, "Failed to establish WebSocket connection.")
		server.users.mutex.Unlock()
		return
	}

	go server.readEventMessages(ws, user.Id)

	went_online := !user.Online
	user.connections += 1
	user.Online = true
	server.users.mutex.Unlock()

	server.conns.mutex.Lock()
	conn := server.conns.add(user.Id)
	connectionCount := len(server.conns.slice)
	server.conns.mutex.Unlock()

	welcome := WelcomeMessage{BuildTime}
	server.writeEventToOneConnection("welcome", welcome, conn)

	player := server.playerGet()
	server.state.mutex.Lock()
	server.users.mutex.Lock()
	messages, _ := server.chatGet(MessageHistoryRequest{100, 0}, user.Id)

	allData := GetAllMessage{
		Users:    server.users.slice,
		Player:   player,
		Playlist: server.state.playlist,
		Messages: messages,
		History:  server.state.history,
	}

	server.writeEventToOneConnection("getall", allData, conn)
	server.state.mutex.Unlock()
	server.users.mutex.Unlock()

	LogInfo("New connection id:%v established with user id:%v on %s. Current connection count: %d", conn.id, user.Id, getIp(r), connectionCount)
	if went_online {
		server.writeEventToAllConnections("userconnected", user.Id, SERVER_ID)
		DatabaseUpdateUserLastOnline(server.db, user.Id, time.Now())
	}

outer:
	for {
		select {
		case event := <-conn.events:
			err := ws.WriteMessage(websocket.TextMessage, event)
			if err != nil {
				LogDebug("Failed to write websocket message: %v", err)
				break outer
			}

		case <-conn.close:
			break outer

		case <-time.After(HEARTBEAT_INTERVAL):
			// NOTE(kihau): Send a heartbeat event to verify that the connection is still active.
			err := ws.WriteMessage(websocket.TextMessage, []byte("{\"type\":\"ping\",\"data\":{}}"))
			if err != nil {
				break outer
			}
		}
	}

	_ = ws.Close()

	server.conns.mutex.Lock()
	server.conns.removeById(conn.id)
	connectionCount = len(server.conns.slice)
	server.conns.mutex.Unlock()

	server.users.mutex.Lock()
	user = server.findUser(token)
	went_offline := false
	if user != nil {
		user.connections -= 1
		went_offline = user.connections == 0
		user.Online = !went_offline

		if went_offline {
			now := time.Now()
			user.LastOnline = now
			DatabaseUpdateUserLastOnline(server.db, user.Id, now)
		}
	}

	server.users.mutex.Unlock()

	if went_offline {
		server.writeEventToAllConnections("userdisconnected", conn.userId, SERVER_ID)
	}

	LogInfo("Connection id:%v of user id:%v dropped. Current connection count: %d", conn.id, conn.userId, connectionCount)
}

func getEventName(eventType EventType) string {
	switch eventType {
	case EVENT_PLAYER_PLAY:
		return "player play"
	case EVENT_PLAYER_PAUSE:
		return "player pause"
	case EVENT_PLAYER_SEEK:
		return "player seek"
	case EVENT_PLAYER_SET:
		return "player set"
	case EVENT_PLAYER_NEXT:
		return "player next"
	case EVENT_PLAYER_AUTOPLAY:
		return "player autoplay"
	case EVENT_PLAYER_LOOPING:
		return "player looping"
	case EVENT_PLAYER_UPDATE_TITLE:
		return "player update title"
	case EVENT_PLAYER_SPEED_CHANGE:
		return "player speed change"

	case EVENT_CHAT_SEND:
		return "chat send"
	case EVENT_CHAT_EDIT:
		return "chat edit"
	case EVENT_CHAT_DELETE:
		return "chat delete"

	case EVENT_PLAYLIST_ADD:
		return "playlist add"
	case EVENT_PLAYLIST_PLAY:
		return "playlist play"
	case EVENT_PLAYLIST_MOVE:
		return "playlist move"
	case EVENT_PLAYLIST_CLEAR:
		return "playlist clear"
	case EVENT_PLAYLIST_DELETE:
		return "playlist delete"
	case EVENT_PLAYLIST_UPDATE:
		return "playlist update"
	case EVENT_PLAYLIST_SHUFFLE:
		return "playlist shuffle"

	default:
		return fmt.Sprintf("<unknown type:%v>", eventType)
	}
}

func handleWsEvent[T any](event WebsocketEvent, userId uint64, handleEvent func(T, uint64) error) {
	eventName := getEventName(event.Type)

	var data T
	err := json.Unmarshal(event.Data, &data)
	if err != nil {
		LogError("Failed to deserialize %v event with data %v: %v", eventName, string(event.Data), err)
		return
	}

	LogInfo("Received WS %v event from userId:%v", eventName, userId)

	err = handleEvent(data, userId)
	if err != nil {
		// NOTE(kihau): We could respond with a custom websocket "error" event (similar to playererror?) to inform the client that something went wrong.
		LogError("Failed to handle %v event: %v", eventName, err)
		return
	}
}

func (server *Server) handleWSMessage(data []byte, userId uint64) {
	var event WebsocketEvent

	if err := json.Unmarshal(data, &event); err != nil {
		LogError("Failed to deserialize WebSocket event: %v", err)
		return
	}

	defer func() {
		if err := recover(); err != nil {
			LogFatalUp(2, "Panic in WebSocket event '%v' serving user id = %v: %v", getEventName(event.Type), userId, err)
		}
	}()

	switch event.Type {
	case EVENT_PLAYER_PLAY:
		handleWsEvent(event, userId, server.playerPlay)

	case EVENT_PLAYER_PAUSE:
		handleWsEvent(event, userId, server.playerPause)

	case EVENT_PLAYER_SEEK:
		handleWsEvent(event, userId, server.playerSeek)

	case EVENT_PLAYER_SET:
		handleWsEvent(event, userId, server.playerSet)

	case EVENT_PLAYER_NEXT:
		handleWsEvent(event, userId, server.playerNext)

	case EVENT_PLAYER_AUTOPLAY:
		handleWsEvent(event, userId, server.playerAutoplay)

	case EVENT_PLAYER_LOOPING:
		handleWsEvent(event, userId, server.playerLooping)

	case EVENT_PLAYER_UPDATE_TITLE:
		handleWsEvent(event, userId, server.playerUpdateTitle)

	case EVENT_PLAYER_SPEED_CHANGE:
		handleWsEvent(event, userId, server.playerSpeedChange)

	case EVENT_CHAT_SEND:
		handleWsEvent(event, userId, server.chatCreate)

	case EVENT_CHAT_EDIT:
		handleWsEvent(event, userId, server.chatEdit)

	case EVENT_CHAT_DELETE:
		handleWsEvent(event, userId, server.chatDelete)

	case EVENT_PLAYLIST_ADD:
		handleWsEvent(event, userId, server.playlistAdd)

	case EVENT_PLAYLIST_PLAY:
		handleWsEvent(event, userId, server.playlistPlay)

	case EVENT_PLAYLIST_MOVE:
		handleWsEvent(event, userId, server.playlistMove)

	case EVENT_PLAYLIST_CLEAR:
		server.playlistClear()

	case EVENT_PLAYLIST_DELETE:
		handleWsEvent(event, userId, server.playlistDelete)

	case EVENT_PLAYLIST_SHUFFLE:
		server.playlistShuffle()

	case EVENT_PLAYLIST_UPDATE:
		handleWsEvent(event, userId, server.playlistUpdate)

	default:
		LogError("Server caught unknown event '%v'", event.Type)
	}
}

func (server *Server) readEventMessages(ws *websocket.Conn, userId uint64) {
	for {
		msgType, data, err := ws.ReadMessage()
		if err != nil {
			break
		}

		if msgType != websocket.TextMessage {
			continue
		}

		go server.handleWSMessage(data, userId)
	}
}
