import yt_dlp
import time
import json
import http.server

class YoutubeVideo:
    def __init__(self, id: str, title: str, thumbnail: str, original_url: str, audio_url: str, video_url: str):
        self.id           = id
        self.title        = title
        self.thumbnail    = thumbnail
        self.original_url = original_url
        self.audio_url    = audio_url
        self.video_url    = video_url


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
        'playliststart': start + 2,
        'playlistend':   end,
    }

    ytplaylist = yt_dlp.YoutubeDL(ytplaylist_opts)
    info = ytplaylist.extract_info(query, download=False)
    print(json.dumps(info, indent=4))

    videos = []

    entries = info['entries']
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
        'format': '(bv*[vcodec~=\'^((he|a)vc|h26[45])\']+ba)',
        
        'extractor_args': {
            'youtube': {
                'player_client': ['ios']
            }
        },
        'playlist_items': '1',
        'noplaylist': True,
    }

    ytfetch = yt_dlp.YoutubeDL(ytfetch_opts)
    info = bench("extract_info", lambda : ytfetch.extract_info(query, download=False))

    if info is None:
        return

    entry = info
    entries = info.get("entries")

    if entries is not None:
        entry = entries[0]

    formats = entry.get("requested_formats")
        
    id           = entry.get("id")
    title        = entry.get("title")
    thumbnail    = entry.get("thumbnail")
    original_url = entry.get("original_url")
    video_url    = formats[0]["url"]
    audio_url    = formats[1]["url"]

    return YoutubeVideo(id, title, thumbnail, original_url, audio_url, video_url)

class InternalServer(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        if self.path == '/youtube/fetch':
            request = json.loads(self.rfile.read1())
            data = bench('get_youtube_video', lambda : get_youtube_video(request["query"]))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)
        elif self.path == '/youtube/playlist':
            request = json.loads(self.rfile.read1())

            data = bench('get_youtube_playlist', lambda : get_youtube_playlist(request["query"], request["start"], request["end"]))
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()

            jsondata = json.dumps(data, default=vars)
            response = bytes(jsondata, "utf-8")
            self.wfile.write(response)

        else:
            self.send_response(404)
            self.end_headers()

hostName = "localhost"
serverPort = 2345

webServer = http.server.ThreadingHTTPServer((hostName, serverPort), InternalServer)
print("Running an internal helper server at http://%s:%s" % (hostName, serverPort))

webServer.serve_forever()
