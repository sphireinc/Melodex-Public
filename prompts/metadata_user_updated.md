Track context:
- File name: {{.FileName}}
- Source kind: {{.SourceKind}}
- Source reference: {{.SourceRef}}
- Source URL: {{.SourceURL}}
- Library root: {{.LibraryRoot}}
- User context: {{.UserContext}}

Source metadata:
- Source title: {{.SourceTitle}}
- Source uploader: {{.SourceUploader}}
- Source channel: {{.SourceChannel}}
- Source description hint: {{.SourceDescriptionHint}}
- Duration seconds: {{.DurationSeconds}}

Existing hints:
- Artist hint: {{.ArtistHint}}
- Album hint: {{.AlbumHint}}
- Title hint: {{.TitleHint}}
- Year hint: {{.YearHint}}
- Genre hint: {{.GenreHint}}
- Track number hint: {{.TrackNumberHint}}

Target library paths:
- Artist directory: {{.TargetArtistDir}}
- Album directory: {{.TargetAlbumDir}}
- Base filename: {{.TargetBaseName}}
- Audio path: {{.TargetAudioPath}}
- Lyrics path: {{.TargetLyricsPath}}
- Timed lyrics path: {{.TargetTimedLyricsPath}}
- Metadata path: {{.TargetMetadataPath}}

Additional notes:
{{.Notes}}

Task:
Infer the best structured metadata and listener-facing enrichment for this track.

Use strong source metadata first.
Use existing hints only as supporting evidence.
Do not treat hints as confirmed facts unless they are clearly supported by source context or reliable external lookup.
Treat the file name as an explicit cleanup hint, especially when it contains source clutter like "(Official HD Video)".

External lookup:
- You may search externally for canonical metadata and enrichment.
- Use external lookup to verify artist, title, album, release year, genre, track number, ISRC, release type, official links, trivia, and song meaning.
- Prefer authoritative sources such as official artist pages, official label pages, platform pages, MusicBrainz, Discogs, AllMusic, Wikipedia/Wikidata, verified YouTube channels, Spotify, Apple Music, and reputable interviews/articles.
- If sources conflict, prefer official release/platform metadata for core fields and mention uncertainty in notes.
- If external lookup is unavailable, blocked, or inconclusive, use only the provided context and leave unsupported fields empty or null.
- Do not include sources that were not actually used.
- Do not include raw search-result pages as sources.

Metadata rules:
- Do not invent artist, album, year, genre, track number, ISRC, release type, links, trivia, or song meaning.
- Prefer the canonical studio album or major release the song belongs to when clearly supported.
- If no major album is supported, fall back to a smaller release such as a single, EP, live release, remix release, soundtrack, or compilation only when clearly supported.
- Do not assume the album from the video title unless clearly supported.
- Do not assume the channel/uploader is the artist unless clearly supported, but check whether it provides useful evidence.
- For official music videos, the video title can strongly support artist and title when the pattern is clear.
- If the source title appears to contain both artist and title, split them only when the pattern is clear.
- Preserve featured artists when clearly present.
- Preserve original casing, punctuation, accents, and stylization when reasonably known.
- If the title is ambiguous after cleanup, preserve the cleaned source title.
- If the artist is ambiguous, leave artist empty.
- If the album is ambiguous, leave album empty.
- If the year is ambiguous, use null.
- If the genre is ambiguous, leave genre empty.
- If the track number is ambiguous, use null.
- Treat user context as a strong clue for correction, but still leave unsupported fields empty rather than inventing facts.

Cleaning guidance:
- Remove obvious platform/source noise from titles, such as:
  "official video", "official music video", "lyrics", "lyric video", "HD", "HQ", "audio", "visualizer", "full album", and similar clutter.
- Preserve meaningful version markers such as:
  "live", "remix", "acoustic", "instrumental", "cover", "demo", "slowed", "nightcore", "karaoke", "radio edit", "remastered", or "explicit" when they identify the recording or release.
- Remove bracketed/parenthetical source clutter only when it is clearly not part of the release title.
- Do not remove featured artist credits or meaningful version markers.

Enrichment rules:
- Provide artist_trivia, album_trivia, song_trivia, song_meaning, tidbits, and links only when supported.
- Keep enrichment useful for a music library UI: concise, interesting, and listener-facing.
- Prefer durable facts over newsy or time-sensitive claims.
- Trivia should be short and factual, not promotional.
- Song meaning should be a concise paraphrase of supported themes, not lyrics and not a deep essay.
- If the meaning is disputed, fan-interpreted, or unsupported, either leave it empty or phrase uncertainty clearly.
- Do not include lyrics.
- Do not include long quotes.
- Do not include rumors, gossip, private personal details, or unsupported claims.
- Do not include non-official social media links.
- Do not include unrelated biographical details that do not help identify or understand the music.

Useful enrichment examples:
- A notable chart achievement when supported.
- A known single/release relationship when supported.
- A notable producer, recording context, or music video fact when supported.
- A concise description of the song's commonly understood themes when supported.
- Official artist website or verified social links when supported.
- Authoritative song/album links when supported.

Important:
- Return only the JSON object requested by the system prompt.
- Do not include lyrics.
- Do not include Markdown.
- Do not include explanatory text outside the JSON object.
