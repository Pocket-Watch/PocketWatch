package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (server *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	uptimeString := fmt.Sprintf("%v_%v", VERSION, BuildTime)
	response, _ := json.Marshal(uptimeString)
	io.WriteString(w, string(response))
}

func (server *Server) apiUptime(w http.ResponseWriter, r *http.Request) {
	uptime := time.Now().Sub(startTime)
	uptimeString := fmt.Sprintf("%v", uptime)
	response, _ := json.Marshal(uptimeString)
	io.WriteString(w, string(response))
}

func (server *Server) apiLogin(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s attempted to log in.", r.RemoteAddr)
	io.WriteString(w, "This is unimplemented")
}

func (server *Server) apiUploadMedia(w http.ResponseWriter, r *http.Request) {
	inputFile, headers, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "Failed to read formdata from the request: %v", err)
		return
	}

	filename := headers.Filename
	// TODO(kihau): Actually check file format by doing http.DetectContentType().
	extension := path.Ext(filename)
	directory := getMediaType(extension)

	outputPath, isSafe := safeJoin("web", "media", directory, filename)
	if !isSafe {
		respondBadRequest(w, "Filename of the uploaded file is not allowed")
		return
	}
	os.MkdirAll("web/media/"+directory, 0750)

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

	networkPath, isSafe := safeJoin("media", directory, filename)
	if !isSafe {
		respondBadRequest(w, "Filename of the uploaded file is not allowed")
		return
	}

	filename = strings.TrimSuffix(filename, extension)
	name := cleanupResourceName(filename)

	response := MediaUploadResponse{
		Url:      networkPath,
		Name:     name,
		Filename: filename,
		Format:   extension,
		Category: directory,
	}

	jsonData, _ := json.Marshal(response)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiUserCreate(w http.ResponseWriter, r *http.Request) {
	server.users.mutex.Lock()
	user := server.users.create()
	DatabaseAddUser(server.db, user)
	server.users.mutex.Unlock()

	tokenJson, err := json.Marshal(user.token)
	if err != nil {
		respondInternalError(w, "Serialization of the user token failed with: %v", err)
		return
	}

	server.writeEventToAllConnections("usercreate", user)
	io.WriteString(w, string(tokenJson))
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

	jsonData, err := json.Marshal(user.Id)
	if err != nil {
		respondInternalError(w, "Serialization of the user id failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
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
	server.writeEventToAllConnections("userdelete", user)

	server.conns.mutex.Lock()
	for _, conn := range server.conns.slice {
		if conn.userId == user.Id {
			conn.close <- true
		}
	}
	server.conns.mutex.Unlock()
}

func (server *Server) apiUserGetAll(w http.ResponseWriter, r *http.Request) {
	server.users.mutex.Lock()
	usersJson, err := json.Marshal(server.users.slice)
	server.users.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the users list failed with: %v", err)
		return
	}

	io.WriteString(w, string(usersJson))
}

func (server *Server) apiUserUpdateName(w http.ResponseWriter, r *http.Request) {
	var newUsername string
	if !server.readJsonDataFromRequest(w, r, &newUsername) {
		return
	}

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

	server.writeEventToAllConnections("userupdate", user)
}

func (server *Server) apiUserUpdateAvatar(w http.ResponseWriter, r *http.Request) {
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

	os.Mkdir("web/users/", os.ModePerm)
	avatarUrl := fmt.Sprintf("web/users/avatar%v", user.Id)

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
	avatarUrl = fmt.Sprintf("users/avatar%v?ext=%v&%v", user.Id, ext, now.UnixMilli())

	server.users.mutex.Lock()
	server.users.slice[userIndex].Avatar = avatarUrl
	server.users.slice[userIndex].LastUpdate = time.Now()
	user = server.users.slice[userIndex]
	DatabaseUpdateUser(server.db, user)
	server.users.mutex.Unlock()

	jsonData, _ := json.Marshal(avatarUrl)

	server.writeEventToAllConnections("userupdate", user)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiPlayerGet(w http.ResponseWriter, r *http.Request) {
	server.state.mutex.Lock()
	player := server.state.player
	server.state.mutex.Unlock()

	player.Timestamp = server.getCurrentTimestamp()

	getEvent := PlayerGetResponse{
		Player: player,
		Entry:  server.state.entry,
		// Subtitles: getSubtitles(),
	}

	jsonData, err := json.Marshal(getEvent)
	if err != nil {
		respondInternalError(w, "Serialization of the get event failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiPlayerSet(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var data PlayerSetRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	entry := Entry{
		Url:        data.RequestEntry.Url,
		UserId:     user.Id,
		Title:      data.RequestEntry.Title,
		UseProxy:   data.RequestEntry.UseProxy,
		RefererUrl: data.RequestEntry.RefererUrl,
		Subtitles:  data.RequestEntry.Subtitles,
	}

	go server.setNewEntry(entry, data.RequestEntry)
}

func (server *Server) apiPlayerEnd(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s reported that video ended.", r.RemoteAddr)

	var data PlaybackEnded
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	defer server.state.mutex.Unlock()

	if data.EntryId == server.state.entry.Id {
		server.state.player.Playing = false
	}
}

func (server *Server) apiPlayerNext(w http.ResponseWriter, r *http.Request) {
	var data PlayerNextRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	if server.state.isLoading.Load() {
		return
	}

	// NOTE(kihau):
	//     Checking whether currently set entry ID on the client side matches current entry ID on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	server.state.mutex.Lock()
	if server.state.entry.Id != data.EntryId {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Entry ID provided in the request is not equal to the current entry ID on the server")
		return
	}

	entry := Entry{}
	if len(server.state.playlist) == 0 && server.state.player.Looping {
		server.state.mutex.Unlock()
		server.playerSeek(0, 0)
		return
	}

	if len(server.state.playlist) != 0 {
		entry = server.playlistRemove(0)
	}
	server.state.mutex.Unlock()

	go server.setNewEntry(entry, RequestEntry{})
}

func (server *Server) apiPlayerPlay(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var data SyncRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.updatePlayerState(true, data.Timestamp)
	event := server.createSyncEvent("play", user.Id)
	server.writeEventToAllConnections("sync", event)
}

func (server *Server) apiPlayerPause(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var data SyncRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.updatePlayerState(false, data.Timestamp)
	event := server.createSyncEvent("pause", user.Id)
	server.writeEventToAllConnections("sync", event)
}

func (server *Server) apiPlayerSeek(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var data SyncRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.playerSeek(data.Timestamp, user.Id)
}

func (server *Server) apiPlayerAutoplay(w http.ResponseWriter, r *http.Request) {
	var autoplay bool
	if !server.readJsonDataFromRequest(w, r, &autoplay) {
		return
	}

	LogInfo("Setting playlist autoplay to %v.", autoplay)

	server.state.mutex.Lock()
	server.state.player.Autoplay = autoplay
	DatabaseSetAutoplay(server.db, autoplay)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("playerautoplay", autoplay)
}

func (server *Server) apiPlayerLooping(w http.ResponseWriter, r *http.Request) {
	var looping bool
	if !server.readJsonDataFromRequest(w, r, &looping) {
		return
	}

	LogInfo("Setting playlist looping to %v.", looping)

	server.state.mutex.Lock()
	server.state.player.Looping = looping
	DatabaseSetLooping(server.db, looping)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("playerlooping", looping)
}

func (server *Server) apiPlayerUpdateTitle(w http.ResponseWriter, r *http.Request) {
	var title string
	if !server.readJsonDataFromRequest(w, r, &title) {
		return
	}

	server.state.mutex.Lock()
	server.state.entry.Title = title
	DatabaseCurrentEntryUpdateTitle(server.db, title)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("playerupdatetitle", title)
}

func (server *Server) apiSubtitleDelete(w http.ResponseWriter, r *http.Request) {
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

	server.writeEventToAllConnections("subtitledelete", subId)
}

func (server *Server) apiSubtitleUpdate(w http.ResponseWriter, r *http.Request) {
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

	server.writeEventToAllConnections("subtitleupdate", data)
}

func (server *Server) apiSubtitleAttach(w http.ResponseWriter, r *http.Request) {
	var subtitle Subtitle
	if !server.readJsonDataFromRequest(w, r, &subtitle) {
		return
	}

	// TODO(kihau): Validate subtitle URL
	subtitle.Id = server.state.subsId.Add(1)

	server.state.mutex.Lock()
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	DatabaseSubtitleAttach(server.db, server.state.entry.Id, subtitle)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("subtitleattach", subtitle)
}

func (server *Server) apiSubtitleShift(w http.ResponseWriter, r *http.Request) {
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

	server.writeEventToAllConnections("subtitleshift", data)
}

func (server *Server) apiSubtitleUpload(w http.ResponseWriter, r *http.Request) {
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

	filename := headers.Filename
	extension := path.Ext(filename)
	subId := server.state.subsId.Add(1)

	outputName := fmt.Sprintf("subtitle%v%v", subId, extension)
	outputPath := path.Join("web", "subs", outputName)
	os.MkdirAll("web/subs/", 0750)

	outputFile, err := os.Create(outputPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer outputFile.Close()

	LogInfo("Saving uploaded subtitle file to: %v.", outputPath)

	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		respondInternalError(w, "Failed to save the subtitle file file: %v", err)
		return
	}

	networkUrl := path.Join("subs", outputName)
	name := strings.TrimSuffix(filename, extension)
	name = cleanupResourceName(name)

	subtitle := Subtitle{
		Id:    subId,
		Name:  name,
		Url:   networkUrl,
		Shift: 0.0,
	}

	jsonData, _ := json.Marshal(subtitle)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiSubtitleDownload(w http.ResponseWriter, r *http.Request) {
	var data SubtitleDownloadRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	url, err := url.Parse(data.Url)
	if err != nil {
		respondBadRequest(w, "Provieded url '%v' is not valid: %v", data.Url, err)
		return
	}

	subId := server.state.subsId.Add(1)
	// Cut at fragment identifier if present
	hash := strings.Index(data.Url, "#")
	if hash >= 0 {
		data.Url = data.Url[:hash]
	}
	filename := filepath.Base(data.Url)
	extension := path.Ext(filename)
	serverUrl := data.Url

	if url.IsAbs() {
		response, err := hastyClient.Get(data.Url)
		if err != nil {
			respondBadRequest(w, "Failed to download subtitle for url %v; %v", data.Url, err)
			return
		}
		defer response.Body.Close()

		if response.StatusCode != http.StatusOK {
			respondBadRequest(w, "Failed to download subtitle for url %v; %v", data.Url, err)
			return
		}

		response.Body = http.MaxBytesReader(w, response.Body, SUBTITLE_SIZE_LIMIT)
		if response.ContentLength > SUBTITLE_SIZE_LIMIT {
			http.Error(w, "Subtitle file is too large", http.StatusRequestEntityTooLarge)
			return
		}

		outputName := fmt.Sprintf("subtitle%v%v", subId, extension)
		outputPath := path.Join("web", "subs", outputName)
		serverUrl = path.Join("subs", outputName)
		os.MkdirAll("web/subs/", 0750)

		outputFile, err := os.Create(outputPath)
		if err != nil {
			respondInternalError(w, "Failed to created file for the subtitle file '%v': %v", data.Url, err)
			return
		}
		defer outputFile.Close()

		_, err = io.Copy(outputFile, response.Body)
		if err != nil {
			respondInternalError(w, "Failed to save downloaded subtitle %v: %v", data.Url, err)
			return
		}
	}

	name := data.Name
	if name == "" {
		name = strings.TrimSuffix(filename, extension)
	}
	name = cleanupResourceName(name)

	subtitle := Subtitle{
		Id:    subId,
		Name:  name,
		Url:   serverUrl,
		Shift: 0.0,
	}

	jsonData, _ := json.Marshal(subtitle)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiSubtitleSearch(w http.ResponseWriter, r *http.Request) {
	if !server.config.EnableSubs {
		http.Error(w, "Feature unavailable", http.StatusServiceUnavailable)
		return
	}

	var search Search
	if !server.readJsonDataFromRequest(w, r, &search) {
		return
	}

	os.MkdirAll("web/media/subs", 0750)
	downloadPath, err := downloadSubtitle(&search, "web/media/subs")
	if err != nil {
		respondBadRequest(w, "Subtitle download failed: %v", err)
		return
	}

	os.MkdirAll("web/subs/", 0750)
	inputSub, err := os.Open(downloadPath)
	if err != nil {
		respondInternalError(w, "Failed to open downloaded subtitle %v: %v", downloadPath, err)
		return
	}
	defer inputSub.Close()

	extension := path.Ext(downloadPath)
	subId := server.state.subsId.Add(1)

	outputName := fmt.Sprintf("subtitle%v%v", subId, extension)
	outputPath := path.Join("web", "subs", outputName)

	outputSub, err := os.Create(outputPath)
	if err != nil {
		respondInternalError(w, "Failed to created output subtitle in %v: %v", outputPath, err)
		return
	}

	defer outputSub.Close()

	_, err = io.Copy(outputSub, inputSub)
	if err != nil {
		respondInternalError(w, "Failed to copy downloaded subtitle file: %v", err)
		return
	}

	outputUrl := path.Join("subs", outputName)
	baseName := filepath.Base(downloadPath)
	subtitleName := strings.TrimSuffix(baseName, extension)
	subtitleName = cleanupResourceName(subtitleName)

	subtitle := Subtitle{
		Id:    subId,
		Name:  subtitleName,
		Url:   outputUrl,
		Shift: 0.0,
	}

	server.state.mutex.Lock()
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections("subtitleattach", subtitle)
}

func (server *Server) apiPlaylistGet(w http.ResponseWriter, r *http.Request) {
	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.playlist)
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the playlist get event failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiPlaylistPlay(w http.ResponseWriter, r *http.Request) {
	var data PlaylistPlayRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	index := FindEntryIndex(server.state.playlist, data.EntryId)
	if index == -1 {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Failed to play playlist element. Entry with ID %v is not in the playlist.", data.EntryId)
		return
	}

	entry := server.playlistRemove(index)
	server.state.mutex.Unlock()

	go server.setNewEntry(entry, RequestEntry{})
}

func (server *Server) apiPlaylistAdd(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var data PlaylistAddRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	requested := data.RequestEntry

	localDirectory, path := server.isLocalDirectory(requested.Url)
	if localDirectory {
		LogInfo("Adding directory '%s' to the playlist.", path)
		localEntries := server.getEntriesFromDirectory(path, user.Id)
		server.state.mutex.Lock()
		server.playlistAddMany(localEntries, requested.AddToTop)
		server.state.mutex.Unlock()
	} else if isYoutube(requested) {
		go server.loadYoutubePlaylist(requested, user.Id)
	} else {
		LogInfo("Adding '%s' url to the playlist.", requested.Url)

		temp := Entry{
			Url:        requested.Url,
			UserId:     user.Id,
			Title:      requested.Title,
			UseProxy:   requested.UseProxy,
			RefererUrl: requested.RefererUrl,
			Subtitles:  requested.Subtitles,
		}

		server.state.mutex.Lock()
		server.playlistAdd(temp, requested.AddToTop)
		server.state.mutex.Unlock()
	}
}

func (server *Server) apiPlaylistClear(w http.ResponseWriter, r *http.Request) {
	server.state.mutex.Lock()
	server.state.playlist = server.state.playlist[:0]
	DatabasePlaylistClear(server.db)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("clear", nil)
	server.writeEventToAllConnections("playlist", event)
}

func (server *Server) apiPlaylistRemove(w http.ResponseWriter, r *http.Request) {
	var data PlaylistRemoveRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	index := FindEntryIndex(server.state.playlist, data.EntryId)
	if index == -1 {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Failed to remove playlist element. Entry with ID %v is not in the playlist.", data.EntryId)
		return
	}

	server.playlistRemove(index)
	server.state.mutex.Unlock()

	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistShuffle(w http.ResponseWriter, r *http.Request) {
	server.state.mutex.Lock()
	for i := range server.state.playlist {
		j := rand.Intn(i + 1)
		server.state.playlist[i], server.state.playlist[j] = server.state.playlist[j], server.state.playlist[i]
	}
	server.state.mutex.Unlock()

	event := createPlaylistEvent("shuffle", server.state.playlist)
	server.writeEventToAllConnections("playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistMove(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var data PlaylistMoveRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	index := FindEntryIndex(server.state.playlist, data.EntryId)
	if index == -1 {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Failed to move playlist element. Entry with ID %v is not in the playlist.", data.EntryId)
		return
	}

	if data.DestIndex < 0 || data.DestIndex >= len(server.state.playlist) {
		respondBadRequest(w, "Failed to move playlist element id:%v. Dest index %v out of bounds.", data.EntryId, data.DestIndex)
		server.state.mutex.Unlock()
		return
	}

	entry := server.state.playlist[index]

	// Remove element from the slice:
	server.state.playlist = slices.Delete(server.state.playlist, index, index+1)

	list := make([]Entry, 0)

	// Appned removed element to a new list:
	list = append(list, server.state.playlist[:data.DestIndex]...)
	list = append(list, entry)
	list = append(list, server.state.playlist[data.DestIndex:]...)

	server.state.playlist = list
	server.state.mutex.Unlock()

	eventData := PlaylistMoveEvent{
		EntryId:   data.EntryId,
		DestIndex: data.DestIndex,
	}

	event := createPlaylistEvent("move", eventData)
	server.writeEventToAllConnections("playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistUpdate(w http.ResponseWriter, r *http.Request) {
	var data PlaylistUpdateRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	index := FindEntryIndex(server.state.playlist, data.Entry.Id)
	if index == -1 {
		server.state.mutex.Unlock()
		respondBadRequest(w, "Entry with id:%v is not in the playlist.", data.Entry.Id)
		return
	}

	updated := server.state.playlist[index]
	updated.Title = data.Entry.Title
	updated.Url = data.Entry.Url
	DatabasePlaylistUpdate(server.db, updated.Id, updated.Title, updated.Url)
	server.state.playlist[index] = updated

	server.state.mutex.Unlock()

	event := createPlaylistEvent("update", updated)
	server.writeEventToAllConnections("playlist", event)
}

func (server *Server) apiHistoryGet(w http.ResponseWriter, r *http.Request) {
	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.history)
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the history get event failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiChatGet(w http.ResponseWriter, r *http.Request) {
	var data MessageHistoryRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	if MAX_CHAT_LOAD < data.Count {
		respondBadRequest(w, "Too many messages were requested.")
		return
	}

	backwardOffset := int(data.BackwardOffset)
	server.state.mutex.Lock()
	availableCount := len(server.state.messages) - backwardOffset
	servedCount := minOf(availableCount, int(data.Count))
	if servedCount <= 0 {
		server.state.mutex.Unlock()
		io.WriteString(w, "[]")
		return
	}
	endOffset := len(server.state.messages) - backwardOffset
	startOffset := endOffset - servedCount
	jsonData, err := json.Marshal(server.state.messages[startOffset:endOffset])
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization failed during chat retrieval: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiChatDelete(w http.ResponseWriter, r *http.Request) {
	var data ChatMessageDeleteRequest
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	user := server.getAuthorized(w, r)
	if user == nil {
		http.Error(w, "No longer authorized", http.StatusUnauthorized)
		return
	}

	server.state.mutex.Lock()
	messages := server.state.messages
	deletedMsgId := -1
	authorMismatch := false
	for b := len(messages) - 1; b >= 0; b-- {
		if messages[b].Id == data.Id {
			msg := &messages[b]
			if msg.AuthorId == user.Id {
				deletedMsgId = int(msg.Id)
				server.state.messages = append(messages[:b], messages[b+1:]...)
				break
			} else {
				authorMismatch = true
				break
			}
		}
	}
	server.state.mutex.Unlock()
	if authorMismatch {
		respondBadRequest(w, "You're not the author of this message")
		return
	}
	if deletedMsgId != -1 {
		server.writeEventToAllConnections("messagedelete", deletedMsgId)
	} else {
		respondBadRequest(w, "No message found of id %v", data.Id)
	}

}

func (server *Server) apiHistoryClear(w http.ResponseWriter, r *http.Request) {
	server.state.mutex.Lock()
	server.state.history = server.state.history[:0]
	server.state.mutex.Unlock()

	DatabaseHistoryClear(server.db)

	server.writeEventToAllConnections("historyclear", nil)
}

func (server *Server) apiHistoryPlay(w http.ResponseWriter, r *http.Request) {
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

	go server.setNewEntry(entry, RequestEntry{})
}

func (server *Server) apiHistoryRemove(w http.ResponseWriter, r *http.Request) {
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

	server.historyRemove(index)
	server.state.mutex.Unlock()
}

func (server *Server) apiChatSend(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s posted a chat message.", r.RemoteAddr)

	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	var newMessage ChatMessageFromUser
	if !server.readJsonDataFromRequest(w, r, &newMessage) {
		return
	}

	if len([]rune(newMessage.Message)) > MAX_MESSAGE_CHARACTERS {
		http.Error(w, "Message exceeds 1000 chars", http.StatusForbidden)
		return
	}

	server.state.mutex.Lock()
	server.state.messageId++
	chatMessage := ChatMessage{
		Id:       server.state.messageId,
		Message:  newMessage.Message,
		AuthorId: user.Id,
		UnixTime: time.Now().UnixMilli(),
		Edited:   false,
	}
	server.state.messages = append(server.state.messages, chatMessage)
	server.state.mutex.Unlock()
	server.writeEventToAllConnections("messagecreate", chatMessage)
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

	went_online := !user.Online
	user.connections += 1
	user.Online = true
	server.users.mutex.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	server.conns.mutex.Lock()
	conn := server.conns.add(user.Id)
	connectionCount := len(server.conns.slice)
	server.conns.mutex.Unlock()

	LogInfo("New connection id:%v established with user id:%v on %s. Current connection count: %d", conn.id, user.Id, r.RemoteAddr, connectionCount)

	// welcomeErr := server.writeEvent(w, "userwelcome", conn.id)
	// if welcomeErr != nil {
	// 	return
	// }

	if went_online {
		server.writeEventToAllConnections("userconnected", user.Id)
		DatabaseUpdateUserLastOnline(server.db, user.Id, time.Now())
	}

outer:
	for {
		select {
		case event := <-conn.events:
			_, err := fmt.Fprint(w, event)
			if err != nil {
				LogDebug("Connection write fail: %v", err)
				break outer
			}

		case <-conn.close:
			break outer

		case <-time.After(HEARTBEAT_INTERVAL):
			// NOTE(kihau): Send a heartbeat event to verify that the connection is still active.
			_, err := w.Write([]byte(":\n\n"))
			if err != nil {
				break outer
			}
		}

		flusher, success := w.(http.Flusher)
		if !success {
			break
		}

		flusher.Flush()
	}

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
		server.writeEventToAllConnections("userdisconnected", conn.userId)
	}

	LogInfo("Connection id:%v of user id:%v dropped. Current connection count: %d", conn.id, conn.userId, connectionCount)
}
