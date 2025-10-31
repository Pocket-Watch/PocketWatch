import yt_dlp
from yt_dlp.utils import (
    DownloadError,
    GeoRestrictedError,
    UnavailableVideoError,
)

import time
import json
import http.server

class YoutubeVideo:
    def __init__(self, 
                 id: str, title: str, thumbnail: str, original_url: str, manifest_url: str, 
                 available_at: int, duration: int, upload_date: str, uploader: str, 
                 artist: str, album: str, release_date: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.manifest_url = manifest_url
        self.available_at = available_at
        self.duration     = duration
        self.upload_date  = upload_date
        self.uploader     = uploader

        # NOTE(kihau): Extra metadata for tracks from YouTube Music. Might not exists, depending on the video.
        self.artist       = artist
        self.album        = album
        self.release_date = release_date


class YoutubePlaylistVideo:
    def __init__(self, url: str, title: str, thumbnails: list):
        self.url          = url
        self.title        = title
        self.thumbnails   = thumbnails

class YoutubePlaylist:
    def __init__(self, entries: list[YoutubePlaylistVideo]):
        self.entries = entries

def get_youtube_playlist(query: str, start: int, end: int):
    ytplaylist_opts = {
        'extract_flat': True,
        'playliststart': start,
        'playlistend':   end,
        'color': 'no_color',
    }

    ytplaylist = yt_dlp.YoutubeDL(ytplaylist_opts)
    info = ytplaylist.extract_info(query, download=False)
    videos = []

    if info is None:
        raise Exception("Yt-Dlp output data is missing")

    entries = info.get('entries')
    if entries is None:
        return YoutubePlaylist([])

    for entry in entries:
        url        = entry["url"]
        title      = entry["title"]
        thumbnails = entry["thumbnails"]
        video      = YoutubePlaylistVideo(url, title, thumbnails)
        videos.append(video)

    return YoutubePlaylist(videos)

def bench(note, func):
    start = time.time()
    data = func()
    end = time.time()
    print(f"[BENCH] {note} elapsed time: " + str(end - start))
    return data

def get_youtube_video(query: str):
    ytfetch_opts = {
        # NOTE(kihau): Only request videos with either H264 or H265 codec.
        # 'format': '(bv*[vcodec~=\'^((he|a)vc|h26[45])\']+ba)',
        
        'extractor_args': {
            'youtube': {
                'player_client': ['web_safari'],
            }
        },
        'playlist_items': '1',
        'noplaylist': True,
        'color': 'no_color',
    }

    ytfetch = yt_dlp.YoutubeDL(ytfetch_opts)

    try:
        info = bench("extract_info", lambda : ytfetch.extract_info(query, download=False))
    except DownloadError as error:
        if error.msg is None:
            return None, "Failed to find YouTube video." 
        else:
            items = error.msg.split(':')[2:]
            error_message = ":".join(items)
            return None, error_message

    except UnavailableVideoError as error:
        return None, "YouTube video is not available." 

    except GeoRestrictedError as error:
        return None, "Failed to load YouTube video due to geo-restriction." 

    except Exception as exception:
        return None, str(exception)

    if info is None:
        return None, "Failed to find YouTube video." 

    entries = info.get("entries")
    entry   = None

    if entries is None:
        entry = info
    elif len(entries) != 0:
        entry = entries[0]

    if entry is None:
        return None, "No YouTube videos found."

    id           = entry.get("id")
    title        = entry.get("title")
    thumbnail    = entry.get("thumbnail")
    original_url = entry.get("original_url")
    manifest_url = entry.get("manifest_url")
    available_at = entry.get("available_at")
    duration     = entry.get("duration")
    upload_date  = entry.get("upload_date")
    uploader     = entry.get("uploader")

    # NOTE(kihau): YouTube Music metadata
    artist       = entry.get("artist")
    album        = entry.get("album")
    release_date = entry.get("release_date")

    if not isinstance(id, str): 
        id = ""

    if not isinstance(title, str):
        title = "Video title is missing :("

    if not isinstance(thumbnail, str):
        thumbnail = ""

    if not isinstance(original_url, str):
        original_url = ""

    if not isinstance(manifest_url, str):
        return None, "Failed to fetch YouTube video. Source URL is missing."

    if not isinstance(available_at, int):
        available_at = 0

    if not isinstance(duration, int):
        duration = 0

    if not isinstance(upload_date, str):
        upload_date = ""

    if not isinstance(uploader, str):
        uploader = ""

    if not isinstance(artist, str):
        artist = ""

    if not isinstance(album, str):
        album = ""

    if not isinstance(release_date, str):
        release_date = ""

    ytVideo = YoutubeVideo(
        id           = id,
        title        = title,
        thumbnail    = thumbnail,
        original_url = original_url,
        manifest_url = manifest_url,
        available_at = available_at,
        duration     = duration,
        upload_date  = upload_date,
        uploader     = uploader,
        artist       = artist,
        album        = album,
        release_date = release_date,
    )

    return ytVideo, None

class TwitchStream:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, url: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.url          = url

def get_twitch_stream(url: str):
    twitch_opts = { 
        'noplaylist': True,
        'color': 'no_color',
    }
    twitch = yt_dlp.YoutubeDL(twitch_opts)
    info = bench("extract_info", lambda : twitch.extract_info(url, download=False))

    if info is None:
        raise Exception("Yt-Dlp did not returned any Twitch streams")

    id           = info.get("id")
    uploader     = info.get("uploader")
    description  = info.get("description")
    thumbnail    = info.get("thumbnail")
    original_url = info.get("original_url")
    source_url   = info.get("url")

    if uploader is None: 
        uploader = "Unknown"

    if not isinstance(description, str): 
        description = "[Stream title is missing]"

    if thumbnail is None: 
        thumbnail = ""

    if not isinstance(original_url, str): 
        original_url = ""

    if source_url is None: 
        source_url = ""

    title = f"Twitch {uploader} (live) - {description}"
    return TwitchStream(id, title, thumbnail, original_url, source_url)

class InternalServer(http.server.BaseHTTPRequestHandler):
    def handle_youtube_request(self, query):
        output, errorMessage = bench('get_youtube_video', lambda : get_youtube_video(query))

        if output is not None:
            self.send_response(200)
            data = output
        else:
            self.send_response(503)
            data = errorMessage

        jsondata = json.dumps(data, default=vars)
        response = bytes(jsondata, "utf-8")

        self.send_header("Content-type", "application/json")
        self.end_headers()

        self.wfile.write(response)

    def handle_request(self):
        if self.path == '/youtube/fetch':
            data = json.loads(self.rfile.read1())
            self.handle_youtube_request(data["query"])

        elif self.path == '/youtube/search':
            data = json.loads(self.rfile.read1())
            query = 'ytsearch:' + data["query"]
            self.handle_youtube_request(query)

        elif self.path == '/youtube/playlist':
            request = json.loads(self.rfile.read1())

            data = bench('get_youtube_playlist', lambda : get_youtube_playlist(request["query"], request["start"], request["end"]))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        elif self.path == '/twitch/fetch':
            url  = json.loads(self.rfile.read1())
            data = bench('get_twitch_stream', lambda : get_twitch_stream(url))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        try:
            self.handle_request()

        except Exception as exception:
            self.send_response(503)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            exception_string = str(exception)
            jsondata = json.dumps(exception_string, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

            raise exception


hostname = "localhost"
port = 2345

web_server = http.server.ThreadingHTTPServer((hostname, port), InternalServer)
print("Running an internal helper server at http://%s:%s" % (hostname, port))

web_server.serve_forever()
