'use strict';

module.exports.transform = function(strData, req, res) {
    if (req.url == '/method/execute.getNewsfeedSmart') {
        var json = JSON.parse(strData);
        var items = json['response']['items'];
        if (items != undefined) {
            var initialLength = items.length;
            for (var i = 0, len = items.length; i < len; i++) {
                if (items[i]['type'] == 'ads' || (items[i]['type'] == 'post' && items[i]['marked_as_ads'] == 1)) {
                    items.splice(i, 1);
                    len--;
                }
            }
            if (initialLength != items.length)
                strData = JSON.stringify(json);
        }
    }
    return strData;
};
