'use strict';

var config = require.main.require('./config');

var escapedAssetsDomain = config.domain.assets.replace('/', '\\/');
var apiReplaces = [
    [ // Ссылки на картинки, музыку и другой контент
        /"https:\\\/\\\/(pu\.vk\.com|[-a-z0-9]+\.(?:userapi\.com|vk-cdn\.net|vkuserlive\.com|vkuservideo\.net|vk\.me))\\\/([^"]+)/g,
        '"https:\\/\\/' + escapedAssetsDomain + '\\/$1\\/$2'
    ],
    [ // Плейлисты для видеозаписей подгружаются с vk.com, в которых нужно подменять ссылки на сами видеозаписи
        /"https:\\\/\\\/vk\.com\\\/(video_hls\.php[^"]+)/g,
        '"https:\\/\\/' + config.domain.api + '\\/vk.com\\/$1'
    ],
    [ // Ссылки на документы и стикеры идут на vk.com
        /"https:\\\/\\\/vk\.com\\\/((images|doc[0-9]+_)[^"]+)/g,
        '"https:\\/\\/' + escapedAssetsDomain + '\\/vk.com\\/$1'
    ],
    [ // Ссылки на превью статей (к примеру - хабр)
        /"preview_url":"https:\\\/\\\/m\.vk\.com\\\/(article[0-9]+)[^"]+"(,"preview_page":"[^"]+",?)?/g,
        ''
    ]
];

var apiUrlReplaces = [{
    selector: '/method/execute',
    replaces: [
        [ // Сервер лонгполла
            '"server":"api.vk.com\\/newuim',
            '"server":"' + config.domain.api + '\\/newuim'
        ]
    ]
}];

var vkcomHls = /https:\/\/([-a-z0-9]+\.(?:vk-cdn\.net|userapi\.com|vkuservideo\.net|vk\.me))\//g;

module.exports.transform = function(response, req, res) {
    let raw = response.raw;
    if (req.url.substr(0, 7) == '/vk.com') {
        // Замена ссылок на видосики (встречаются в плейлистах video_hls.php)
        raw = raw.replace(vkcomHls, 'https://' + config.domain.assets + '/$1/');

    } else {
        for (var i = 0, len = apiReplaces.length; i < len; i++)
            raw = raw.replace(apiReplaces[i][0], apiReplaces[i][1]);

        for (var i = 0, len = apiUrlReplaces.length; i < len; i++) {
            let entry = apiUrlReplaces[i];
            if (req.url == entry.selector || (entry.selectorRegex != undefined && entry.selectorRegex.test(req.url))) {
                for (var i = 0, len = entry.replaces.length; i < len; i++)
                    raw = raw.replace(entry.replaces[i][0], entry.replaces[i][1]);
            }
        }
    }
    response.raw = raw;
};
