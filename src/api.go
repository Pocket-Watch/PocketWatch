package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (server *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested server version.", r.RemoteAddr)
	uptimeString := fmt.Sprintf("%v_%v", VERSION, BuildTime)
	response, _ := json.Marshal(uptimeString)
	io.WriteString(w, string(response))
}

func (server *Server) apiUptime(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested server version.", r.RemoteAddr)
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

	extension := path.Ext(headers.Filename)
	directory := getMediaType(extension)
	filename := headers.Filename

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

	jsonData, _ := json.Marshal(networkPath)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiUserCreate(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user creation.", r.RemoteAddr)

	server.users.mutex.Lock()
	user := server.users.create()
	DatabaseAddUser(server.db, user)
	server.users.mutex.Unlock()

	tokenJson, err := json.Marshal(user.token)
	if err != nil {
		respondInternalError(w, "Serialization of the user token failed with: %v", err)
		return
	}

	server.writeEventToAllConnections(w, "usercreate", user)
	io.WriteString(w, string(tokenJson))
}

func (server *Server) apiUserVerify(w http.ResponseWriter, r *http.Request) {
	var token string
	if !server.readJsonDataFromRequest(w, r, &token) {
		return
	}

	user := server.findUser(token)
	if user == nil {
		respondBadRequest(w, "User with specified token was not found")
		return
	}

	LogInfo("Connection requested %s user verification.", r.RemoteAddr)

	jsonData, err := json.Marshal(user.Id)
	if err != nil {
		respondInternalError(w, "Serialization of the user id failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiUserDelete(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user deletion.", r.RemoteAddr)

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
	server.writeEventToAllConnections(w, "userdelete", user)
}

func (server *Server) apiUserGetAll(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user get all.", r.RemoteAddr)

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
	LogInfo("Connection requested %s user name change.", r.RemoteAddr)

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
	server.users.slice[userIndex].lastUpdate = time.Now()
	user := server.users.slice[userIndex]
	DatabaseUpdateUser(server.db, user)
	server.users.mutex.Unlock()

	server.writeEventToAllConnections(w, "userupdate", user)
}

func (server *Server) apiUserUpdateAvatar(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection requested %s user avatar change.", r.RemoteAddr)

	server.users.mutex.Lock()
	userIndex := server.getAuthorizedIndex(w, r)
	if userIndex == -1 {
		server.users.mutex.Unlock()
		return
	}
	user := server.users.slice[userIndex]
	server.users.mutex.Unlock()

	formfile, _, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "Failed to read form data from the user avatar change request: %v", err)
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
	io.Copy(file, formfile)

	now := time.Now()
	avatarUrl = fmt.Sprintf("users/avatar%v?%v", user.Id, now.UnixMilli())

	server.users.mutex.Lock()
	server.users.slice[userIndex].Avatar = avatarUrl
	server.users.slice[userIndex].lastUpdate = time.Now()
	user = server.users.slice[userIndex]
	DatabaseUpdateUser(server.db, user)
	server.users.mutex.Unlock()

	jsonData, _ := json.Marshal(avatarUrl)

	server.writeEventToAllConnections(w, "userupdate", user)
	io.WriteString(w, string(jsonData))
}

func (server *Server) apiPlayerGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested get.", r.RemoteAddr)

	server.state.mutex.Lock()
	getEvent := PlayerGetResponseData{
		Player: server.state.player,
		Entry:  server.state.entry,
		// Subtitles: getSubtitles(),
	}
	server.state.mutex.Unlock()

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

	LogInfo("Connection %s requested media url change.", r.RemoteAddr)

	var data PlayerSetRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	server.state.entryId += 1
	id := server.state.entryId
	server.state.mutex.Unlock()

	newEntry := Entry{
		Id:         id,
		Url:        data.RequestEntry.Url,
		UserId:     user.Id,
		Title:      data.RequestEntry.Title,
		UseProxy:   data.RequestEntry.UseProxy,
		RefererUrl: data.RequestEntry.RefererUrl,
		SourceUrl:  "",
		Subtitles:  data.RequestEntry.Subtitles,
		Created:    time.Now(),
	}

	newEntry.Title = constructTitleWhenMissing(&newEntry)

	server.loadYoutubeEntry(&newEntry, data.RequestEntry)

	server.state.mutex.Lock()
	if server.state.entry.Url != "" && server.state.player.Looping {
		server.state.playlist = append(server.state.playlist, server.state.entry)
	}

	prevEntry := server.setNewEntry(&newEntry)
	server.state.mutex.Unlock()

	LogInfo("New url is now: '%s'.", server.state.entry.Url)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	server.writeEventToAllConnections(w, "playerset", setEvent)
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
	LogInfo("Connection %s requested playlist next.", r.RemoteAddr)

	var data PlayerNextRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	// NOTE(kihau):
	//     We need to check whether currently set entry ID on the client side matches current entry ID on the server side.
	//     This check is necessary because multiple clients can send "playlist next" request on video end,
	//     resulting in multiple playlist skips, which is not an intended behaviour.

	if server.state.entry.Id != data.EntryId {
		respondBadRequest(w, "Entry ID provided in the request is not equal to the current entry ID on the server")
		return
	}

	server.state.mutex.Lock()
	if server.state.entry.Url != "" && server.state.player.Looping {
		server.state.playlist = append(server.state.playlist, server.state.entry)
	}

	newEntry := Entry{}
	if len(server.state.playlist) != 0 {
		newEntry = server.state.playlist[0]
		server.state.playlist = server.state.playlist[1:]
	}

	server.loadYoutubeEntry(&newEntry, RequestEntry{})
	prevEntry := server.setNewEntry(&newEntry)
	server.state.mutex.Unlock()

	nextEvent := PlayerNextEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	server.writeEventToAllConnections(w, "playernext", nextEvent)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlayerPlay(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player start.", r.RemoteAddr)

	var data SyncRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.updatePlayerState(true, data.Timestamp)
	event := server.createSyncEvent("play", user.Id)
	server.writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func (server *Server) apiPlayerPause(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player pause.", r.RemoteAddr)

	var data SyncRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.updatePlayerState(false, data.Timestamp)
	event := server.createSyncEvent("pause", user.Id)
	server.writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func (server *Server) apiPlayerSeek(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested player seek.", r.RemoteAddr)

	var data SyncRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	server.state.player.Timestamp = data.Timestamp
	server.state.lastUpdate = time.Now()
	server.state.mutex.Unlock()

	event := server.createSyncEvent("seek", user.Id)

	io.WriteString(w, "Broadcasting seek!\n")
	server.writeEventToAllConnectionsExceptSelf(w, "sync", event, user.Id, data.ConnectionId)
}

func (server *Server) apiPlayerAutoplay(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist autoplay.", r.RemoteAddr)

	var autoplay bool
	if !server.readJsonDataFromRequest(w, r, &autoplay) {
		return
	}

	LogInfo("Setting playlist autoplay to %v.", autoplay)

	server.state.mutex.Lock()
	server.state.player.Autoplay = autoplay
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "playerautoplay", autoplay)
}

func (server *Server) apiPlayerLooping(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist looping.", r.RemoteAddr)

	var looping bool
	if !server.readJsonDataFromRequest(w, r, &looping) {
		return
	}

	LogInfo("Setting playlist looping to %v.", looping)

	server.state.mutex.Lock()
	server.state.player.Looping = looping
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "playerlooping", looping)
}

func (server *Server) apiPlayerUpdateTitle(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested title update.", r.RemoteAddr)

	var title string
	if !server.readJsonDataFromRequest(w, r, &title) {
		return
	}

	server.state.mutex.Lock()
	server.state.entry.Title = title
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "playerupdatetitle", title)
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
			break
		}
	}
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitledelete", subId)
}

func (server *Server) apiSubtitleUpdate(w http.ResponseWriter, r *http.Request) {
	var data SubtitleUpdateRequestData
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
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleupdate", data)
}

func (server *Server) apiSubtitleAttach(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested attach sub.", r.RemoteAddr)

	var subtitle Subtitle
	if !server.readJsonDataFromRequest(w, r, &subtitle) {
		return
	}

	server.state.mutex.Lock()
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleattach", subtitle)
}

func (server *Server) apiSubtitleShift(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested attach sub.", r.RemoteAddr)

	var data SubtitleShiftRequestData
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
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleshift", data)
}

func (server *Server) apiSubtitleUpload(w http.ResponseWriter, r *http.Request) {
	// Ensure that file content doesn't exceed maximum subtitle size limit.
	r.Body = http.MaxBytesReader(w, r.Body, SUBTITLE_SIZE_LIMIT)

	networkFile, headers, err := r.FormFile("file")
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

	// NOTE(kihau): Maybe instead of io.ReadAll, a writer to a file should be used?
	data, err := io.ReadAll(networkFile)
	if err != nil {
		respondInternalError(w, "Failed to read downloaded subtitle file: %v", err)
		return
	}

	_, err = outputFile.Write(data)
	if err != nil {
		respondInternalError(w, "Subtitle file write failed with: %v", err)
		return
	}

	networkUrl := path.Join("subs", outputName)
	subtitle := Subtitle{
		Id:    subId,
		Name:  strings.TrimSuffix(filename, extension),
		Url:   networkUrl,
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

	subtitle := Subtitle{
		Id:    subId,
		Name:  subtitleName,
		Url:   outputUrl,
		Shift: 0.0,
	}

	server.state.mutex.Lock()
	server.state.entry.Subtitles = append(server.state.entry.Subtitles, subtitle)
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "subtitleattach", subtitle)
}

func (server *Server) apiPlaylistGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist get.", r.RemoteAddr)

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
	var data PlaylistPlayRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	LogInfo("Connection %s requested playlist play.", r.RemoteAddr)

	server.state.mutex.Lock()
	if data.Index < 0 || data.Index >= len(server.state.playlist) {
		respondBadRequest(w, "Failed to play playlist element %v. Index out of bounds.", data.Index)
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[data.Index].Id != data.EntryId {
		respondBadRequest(w, "Playlist entry ID provided in the request is not equal to the playlist entry ID on the server")
		server.state.mutex.Unlock()
		return
	}

	if server.state.entry.Url != "" && server.state.player.Looping {
		server.state.playlist = append(server.state.playlist, server.state.entry)
	}

	newEntry := server.state.playlist[data.Index]
	server.loadYoutubeEntry(&newEntry, RequestEntry{})
	prevEntry := server.setNewEntry(&newEntry)
	server.state.playlist = slices.Delete(server.state.playlist, data.Index, data.Index+1)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("remove", data.Index)
	server.writeEventToAllConnections(w, "playlist", event)

	setEvent := PlayerSetEventData{
		PrevEntry: prevEntry,
		NewEntry:  newEntry,
	}
	server.writeEventToAllConnections(w, "playerset", setEvent)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistAdd(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist add.", r.RemoteAddr)

	var data PlaylistAddRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	localDir, path := server.isLocalDirectory(data.RequestEntry.Url)
	if data.RequestEntry.IsPlaylist && localDir {
		LogInfo("Adding directory '%s' to the playlist.", path)
		localEntries := server.getEntriesFromDirectory(path, user.Id)

		server.state.mutex.Lock()
		server.state.playlist = append(server.state.playlist, localEntries...)
		server.state.mutex.Unlock()

		event := createPlaylistEvent("addmany", localEntries)
		server.writeEventToAllConnections(w, "playlist", event)
	} else {
		LogInfo("Adding '%s' url to the playlist.", data.RequestEntry.Url)

		server.state.mutex.Lock()
		server.state.entryId += 1
		id := server.state.entryId
		server.state.mutex.Unlock()

		newEntry := Entry{
			Id:         id,
			Url:        data.RequestEntry.Url,
			UserId:     user.Id,
			Title:      data.RequestEntry.Title,
			UseProxy:   data.RequestEntry.UseProxy,
			RefererUrl: data.RequestEntry.RefererUrl,
			SourceUrl:  "",
			Subtitles:  data.RequestEntry.Subtitles,
			Created:    time.Now(),
		}

		newEntry.Title = constructTitleWhenMissing(&newEntry)

		server.loadYoutubeEntry(&newEntry, data.RequestEntry)

		server.state.mutex.Lock()
		server.state.playlist = append(server.state.playlist, newEntry)
		server.state.mutex.Unlock()

		event := createPlaylistEvent("add", newEntry)
		server.writeEventToAllConnections(w, "playlist", event)
	}
}

func (server *Server) apiPlaylistClear(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist clear.", r.RemoteAddr)

	var connectionId uint64
	if !server.readJsonDataFromRequest(w, r, &connectionId) {
		return
	}

	server.state.mutex.Lock()
	server.state.playlist = server.state.playlist[:0]
	server.state.mutex.Unlock()

	event := createPlaylistEvent("clear", nil)
	server.writeEventToAllConnections(w, "playlist", event)
}

func (server *Server) apiPlaylistRemove(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist remove.", r.RemoteAddr)

	var data PlaylistRemoveRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	server.state.mutex.Lock()
	if data.Index < 0 || data.Index >= len(server.state.playlist) {
		respondBadRequest(w, "Failed to remove playlist element %v. Index out of bounds.", data.Index)
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[data.Index].Id != data.EntryId {
		respondBadRequest(w, "Playlist entry ID provided in the request is not equal to the playlist entry ID on the server")
		server.state.mutex.Unlock()
		return
	}

	server.state.playlist = slices.Delete(server.state.playlist, data.Index, data.Index+1)
	server.state.mutex.Unlock()

	event := createPlaylistEvent("remove", data.Index)
	server.writeEventToAllConnections(w, "playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistShuffle(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested playlist shuffle.", r.RemoteAddr)

	server.state.mutex.Lock()
	for i := range server.state.playlist {
		j := rand.Intn(i + 1)
		server.state.playlist[i], server.state.playlist[j] = server.state.playlist[j], server.state.playlist[i]
	}
	server.state.mutex.Unlock()

	event := createPlaylistEvent("shuffle", server.state.playlist)
	server.writeEventToAllConnections(w, "playlist", event)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistMove(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist move.", r.RemoteAddr)

	var move PlaylistMoveRequestData
	if !server.readJsonDataFromRequest(w, r, &move) {
		return
	}

	server.state.mutex.Lock()
	if move.SourceIndex < 0 || move.SourceIndex >= len(server.state.playlist) {
		respondBadRequest(w, "Failed to move playlist element %v. Source index out of bounds.", move.SourceIndex)
		server.state.mutex.Unlock()
		return
	}

	if server.state.playlist[move.SourceIndex].Id != move.EntryId {
		respondBadRequest(w, "Playlist entry ID provided in the move request is not equal to the playlist entry ID on the server")
		server.state.mutex.Unlock()
		return
	}

	if move.DestIndex < 0 || move.DestIndex >= len(server.state.playlist) {
		respondBadRequest(w, "Failed to move playlist element %v. Dest index %v out of bounds.", move.SourceIndex, move.DestIndex)
		server.state.mutex.Unlock()
		return
	}

	entry := server.state.playlist[move.SourceIndex]

	// Remove element from the slice:
	server.state.playlist = slices.Delete(server.state.playlist, move.SourceIndex, move.SourceIndex+1)

	list := make([]Entry, 0)

	// Appned removed element to a new list:
	list = append(list, server.state.playlist[:move.DestIndex]...)
	list = append(list, entry)
	list = append(list, server.state.playlist[move.DestIndex:]...)

	server.state.playlist = list
	server.state.mutex.Unlock()

	eventData := PlaylistMoveEventData{
		SourceIndex: move.SourceIndex,
		DestIndex:   move.DestIndex,
	}

	event := createPlaylistEvent("move", eventData)
	server.writeEventToAllConnectionsExceptSelf(w, "playlist", event, user.Id, move.ConnectionId)
	go server.preloadYoutubeSourceOnNextEntry()
}

func (server *Server) apiPlaylistUpdate(w http.ResponseWriter, r *http.Request) {
	user := server.getAuthorized(w, r)
	if user == nil {
		return
	}

	LogInfo("Connection %s requested playlist update.", r.RemoteAddr)

	var data PlaylistUpdateRequestData
	if !server.readJsonDataFromRequest(w, r, &data) {
		return
	}

	entry := data.Entry

	server.state.mutex.Lock()
	updatedEntry := Entry{Id: 0}

	for i := range server.state.playlist {
		if server.state.playlist[i].Id == entry.Id {
			server.state.playlist[i].Title = entry.Title
			server.state.playlist[i].Url = entry.Url
			updatedEntry = server.state.playlist[i]
			break
		}
	}

	server.state.mutex.Unlock()

	if updatedEntry.Id == 0 {
		LogWarn("Failed to find entry to update")
		return
	}

	event := createPlaylistEvent("update", entry)
	server.writeEventToAllConnectionsExceptSelf(w, "playlist", event, user.Id, data.ConnectionId)
}

func (server *Server) apiHistoryGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested history get.", r.RemoteAddr)

	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.history)
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the history get event failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
}

func (server *Server) apiHistoryClear(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested history clear.", r.RemoteAddr)

	server.state.mutex.Lock()
	server.state.history = server.state.history[:0]
	server.state.mutex.Unlock()

	server.writeEventToAllConnections(w, "historyclear", nil)
}

func (server *Server) apiChatGet(w http.ResponseWriter, r *http.Request) {
	LogInfo("Connection %s requested messages.", r.RemoteAddr)

	server.state.mutex.Lock()
	jsonData, err := json.Marshal(server.state.messages)
	server.state.mutex.Unlock()

	if err != nil {
		respondInternalError(w, "Serialization of the chat get event failed with: %v", err)
		return
	}

	io.WriteString(w, string(jsonData))
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
	if len(newMessage.Message) > MAX_MESSAGE_CHARACTERS {
		http.Error(w, "Message exceeds 1000 chars", http.StatusForbidden)
		return
	}

	server.state.mutex.Lock()
	server.state.messageId += 1
	chatMessage := ChatMessage{
		Id:       1,
		Message:  newMessage.Message,
		AuthorId: user.Id,
		UnixTime: time.Now().UnixMilli(),
		Edited:   false,
	}
	server.state.messages = append(server.state.messages, chatMessage)
	server.state.mutex.Unlock()
	server.writeEventToAllConnections(w, "messagecreate", chatMessage)
}

func (server *Server) apiEvents(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		respondBadRequest(w, "Failed to connect to event stream. User token missing")
		return
	}

	server.users.mutex.Lock()
	userIndex := server.users.findByToken(token)
	if userIndex == -1 {
		respondBadRequest(w, "Failed to connect to event stream. User with specified token not found.")
		server.users.mutex.Unlock()
		return
	}

	userId := server.users.slice[userIndex].Id
	went_online := server.users.addConnection(userId)
	if went_online {
		DatabaseUpdateUserLastOnline(server.db, userId, time.Now())
	}
	server.users.mutex.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	server.conns.mutex.Lock()
	conn := server.conns.add(userId)
	connectionCount := len(server.conns.slice)
	server.conns.mutex.Unlock()

	LogInfo("New connection id:%v established with user id:%v on %s. Current connection count: %d", conn.id, userId, r.RemoteAddr, connectionCount)

	welcomeErr := server.writeEvent(w, "userwelcome", conn.id)
	if welcomeErr != nil {
		return
	}

	if went_online {
		server.writeEventToAllConnectionsExceptSelf(w, "userconnected", userId, userId, conn.id)
		DatabaseUpdateUserLastOnline(server.db, userId, time.Now())
	}

	for {
		event := <-conn.events
		_, err := fmt.Fprint(w, event)

		if err != nil {
			LogDebug("Connection write fail: %v", err)
			break
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
	went_offline := server.users.removeConnection(conn.userId)
	server.users.mutex.Unlock()

	if went_offline {
		server.writeEventToAllConnections(nil, "userdisconnected", conn.userId)
	}

	LogInfo("Connection id:%v of user id:%v dropped. Current connection count: %d", conn.id, conn.userId, connectionCount)
}
