package main

// Constants
var (
	apiOfficialLongpollPath  = []byte("/method/execute")
	apiOfficialLongpollPath2 = []byte("/method/execute.imGetLongPollHistoryExtended")
	apiOfficialLongpollPath3 = []byte("/method/execute.imLpInit")
	apiOfficialNewsfeedPath  = []byte("/method/execute.getNewsfeedSmart")
	apiLongpollPath          = []byte("/method/messages.getLongPollServer")
	apiNewsfeedGet           = []byte("/method/newsfeed.get")
	videoHlsPath             = []byte("/video_hls.php")
	atPath                   = []byte("/%40")
	awayPath                 = []byte("/away")

	https        = []byte("https")
	apiHost      = []byte("api.vk.com")
	siteHost     = []byte("vk.com")
	siteHostRoot = []byte(".vk.com")

	gzip            = []byte("gzip")
	setCookie       = []byte("Set-Cookie")
	acceptEncoding  = []byte("Accept-Encoding")
	contentEncoding = []byte("Content-Encoding")
)
