'use strict';

module.exports.transform = function(response, req, res) {
    if (req.url == '/method/execute.getNewsfeedSmart') {
        var json = response.json;
        if ('response' in json && 'items' in json['response']) {
            var items = json['response']['items'];
            var initialLength = items.length;
            for (var i = 0, len = items.length; i < len; i++) {
                if (items[i]['type'] == 'ads' || (items[i]['type'] == 'post' && items[i]['marked_as_ads'] == 1)) {
                    items.splice(i, 1);
                    len--;
                }
            }
            if (initialLength != items.length)
                response.json = json;
        }
    } else if (req.url == '/method/execute.getCountersAndInfo') {
        var modified = false;
        var json = response.json;
        if ('response' in json) {
            var response = json['response'];
            if ('audio_ads' in response) {
                response['audio_ads']['day_limit'] = 0;
                response['audio_ads']['types_allowed'] = [];
                response['audio_ads']['sections'] = [];
            }
            if ('profiler_enabled' in response)
                response['profiler_enabled'] = false;
            if ('settings' in response) {
                var settings = response['settings'];
                for (var i = 0, len = settings.length; i < len; i++) {
                    if (['audio_ads', 'audio_restrictions'].includes(settings[i]['name'])) {
                        settings[i]['available'] = false;
                        modified = true;
                    }
                }
            }
            if (modified)
                response.json = json;
        }
    }
};
