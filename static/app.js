// Activity arcade: fetches /api/activity, renders the board, filters by chip,
// loads deeper pages up to GitHub's memory (10 pages, 90 days).
(function () {
  'use strict';

  var form = document.getElementById('fetch-form');
  var input = document.getElementById('username');
  var button = form.querySelector('button[type="submit"]');
  var help = document.getElementById('fetch-help');
  var result = document.getElementById('result');

  var state = { user: null, events: [], stats: null, page: 0, hasMore: false, filter: 'all' };

  var CATS = {
    all: 'All',
    push: 'Pushes',
    star: 'Stars',
    pull: 'Pulls',
    issue: 'Issues',
    release: 'Releases',
    comment: 'Comments',
    repo: 'Repos',
    other: 'Other'
  };

  function esc(s) {
    var el = document.createElement('span');
    el.textContent = s == null ? '' : String(s);
    return el.innerHTML;
  }

  function setHelp(msg, isError) {
    help.classList.toggle('is-error', !!isError);
    help.textContent = msg || '';
  }

  function renderStats() {
    var s = state.stats;
    if (!s) return '';
    var pushes = s.pushes != null ? s.pushes : s.commits;
    return '<div class="stats" aria-label="Score line">' +
      cell(s.events, 'events') + cell(pushes, 'pushes') +
      cell(s.stars, 'stars given') + cell(s.pulls, 'pull requests') +
      cell(s.issues, 'issues') + cell(s.repos, 'repos touched') +
      '</div>';
  }

  function cell(n, label) {
    return '<div><b>' + Number(n).toLocaleString() + '</b><span>' + label + '</span></div>';
  }

  function renderChips() {
    var counts = { all: state.events.length };
    state.events.forEach(function (e) {
      counts[e.category] = (counts[e.category] || 0) + 1;
    });
    var names = ['all'].concat(Object.keys(CATS).filter(function (k) {
      return k !== 'all' && counts[k];
    }));
    if (!names.length) return '';

    var chips = names.map(function (cat) {
      var checked = state.filter === cat ? ' checked' : '';
      return '<input type="radio" name="cat" id="cat-' + cat + '" value="' + cat + '"' + checked + '>' +
        '<label for="cat-' + cat + '">' + CATS[cat] + ' <span>' + counts[cat] + '</span></label>';
    });
    return '<fieldset class="seg" id="chips"><legend class="visually-hidden">Filter the feed by event type</legend>' + chips + '</fieldset>';
  }

  function renderFeed() {
    var shown = state.events.filter(function (e) {
      return state.filter === 'all' || e.category === state.filter;
    });
    if (!shown.length) {
      return '<div class="note"><p>No events of this kind on the board.</p><p>Try the All chip, or fetch another username.</p></div>';
    }
    var rows = shown.map(function (e, i) {
      var isNew = state.filter === 'all' && i === 0;
      return '<a class="row' + (isNew ? ' is-new' : '') + '" href="' + esc(e.url) + '" rel="noopener">' +
        '<span class="row__time">' + esc(e.time) + '<span class="row__date">' + esc(e.date) + '</span></span>' +
        '<span class="row__text"><span class="row__verb">' + esc(e.sentence) + '</span> <span class="row__repo">' + esc(e.repo) + '</span></span>' +
        '</a>';
    });
    return '<div class="feed">' + rows.join('') + '</div>';
  }

  function renderMore() {
    if (!state.hasMore || state.filter !== 'all') return '';
    return '<div class="more"><button class="btn btn--quiet" type="button" id="more">Load 30 more</button></div>';
  }

  function renderUser() {
    var u = state.user;
    if (!u) return '';
    var name = u.name && u.name !== u.login ? '<span class="user__name">' + esc(u.name) + '</span>' : '';
    return '<div class="user">' +
      '<img class="user__avatar" src="' + esc(u.avatar_url) + '" width="56" height="56" alt="">' +
      '<p><a href="' + esc(u.url) + '" rel="noopener"><span class="user__login">' + esc(u.login) + '</span></a>' + name + '</p>' +
      '</div>';
  }

  function renderBoard() {
    if (state.error) return '';
    return renderUser() + renderStats() + renderChips() + renderFeed() + renderMore();
  }

  function paint() {
    if (state.loading) { result.innerHTML = skeletons(6); return; }
    if (state.error) { result.innerHTML = errorBox(state.error); return; }
    if (!state.user) { result.innerHTML = emptyBox(); return; }
    result.innerHTML = renderBoard();
    bindChips();
    bindMore();
  }

  function skeletons(n) {
    var rows = '';
    for (var i = 0; i < n; i++) {
      rows += '<div class="sk" aria-hidden="true"><i></i><i></i></div>';
    }
    return '<p class="visually-hidden" role="status">Fetching the last 30 events from GitHub.</p><div class="feed">' + rows + '</div>';
  }

  function errorBox(msg) {
    return '<div class="note is-error" role="status"><p>' + esc(msg) + '</p><p>Nothing on the board changed. Fix the problem above and fetch again.</p></div>';
  }

  function emptyBox() {
    return '<div class="empty"><p class="empty__title">No games on the board yet.</p>' +
      '<p class="empty__line">Every score comes from the public GitHub events feed. Drop a username above and press fetch.</p></div>';
  }

  function bindChips() {
    var box = document.getElementById('chips');
    if (!box) return;
    box.addEventListener('change', function (e) {
      if (e.target.name === 'cat') {
        state.filter = e.target.value;
        paint();
      }
    });
  }

  function bindMore() {
    var btn = document.getElementById('more');
    if (!btn) return;
    btn.addEventListener('click', function () {
      btn.disabled = true;
      btn.setAttribute('aria-busy', 'true');
      btn.textContent = 'Loading…';
      fetch('/api/activity?username=' + encodeURIComponent(state.user.login) + '&page=' + (state.page + 1), { headers: { 'Accept': 'application/json' } })
        .then(function (r) { return r.json().then(function (b) { return { ok: r.ok, status: r.status, body: b }; }); })
        .then(function (res) {
          if (res.ok) {
            state.user = res.body.user;
            state.events = res.body.events || [];
            state.stats = res.body.stats;
            state.page = res.body.page;
            state.hasMore = !!res.body.has_more;
            state.error = null;
            paint();
          } else {
            state.error = (res.body && res.body.error) || 'GitHub answered something unexpected. Try again in a minute.';
            setHelp('Fetch failed. The board below explains.', true);
            paint();
          }
        })
        .catch(function () {
          state.error = 'Could not reach this server. Check the connection and try again.';
          setHelp('Fetch failed. The board below explains.', true);
          paint();
        });
    });
  }

  function fetchUser(username) {
    state.loading = true;
    state.error = null;
    state.filter = 'all';
    paint();
    button.setAttribute('aria-busy', 'true');
    button.disabled = true;

    fetch('/api/activity?username=' + encodeURIComponent(username), { headers: { 'Accept': 'application/json' } })
      .then(function (r) { return r.json().then(function (b) { return { ok: r.ok, status: r.status, body: b }; }); })
      .then(function (res) {
        state.loading = false;
        if (res.ok) {
          state.user = res.body.user;
          state.events = res.body.events || [];
          state.stats = res.body.stats;
          state.page = res.body.page;
          state.hasMore = !!res.body.has_more;
          setHelp('Fetched from the public events feed, newest first. GitHub keeps 90 days and 300 events; Load 30 more walks that memory.');
          paint();
        } else {
          state.error = (res.body && res.body.error) || 'GitHub answered something unexpected. Try again in a minute.';
          setHelp('Fetch failed. The board below explains.', true);
          if (res.status === 400) {
            input.setAttribute('aria-invalid', 'true');
            setHelp(state.error, true);
            input.focus();
          }
          paint();
        }
      })
      .catch(function () {
        state.loading = false;
        state.error = 'Could not reach this server. Check the connection and try again.';
        setHelp('Fetch failed. The board below explains.', true);
        paint();
      })
      .finally(function () {
        button.removeAttribute('aria-busy');
        button.disabled = false;
      });
  }

  form.addEventListener('submit', function (e) {
    e.preventDefault();
    var username = input.value.trim();
    input.removeAttribute('aria-invalid');
    setHelp('', false);

    if (!username) {
      input.setAttribute('aria-invalid', 'true');
      setHelp('Write a username first, then press fetch.', true);
      input.focus();
      return;
    }
    if (!input.checkValidity()) {
      input.setAttribute('aria-invalid', 'true');
      setHelp('That is not a valid GitHub username. Letters, numbers and hyphens only, up to 39 characters.', true);
      input.focus();
      // The board keeps whatever it held; the problem is named here alone.
      if (state.error) { state.error = null; paint(); }
      return;
    }

    fetchUser(username);
  });
})();
