# HALO Prompt Files

This directory contains the editable prompt text used by `halo` when it calls the language model.

The code still adds runtime context in code, such as timestamps, host details, topic seeds, target word counts, aware-memory summaries, aggregated previous outputs, and canon summary text. These files hold the durable instruction layers that are intended to be tuned directly.

## Files

- `halo-persona.txt`: shared persona base for story generation, aware generation, and review generation.
- `storygen-system.txt`: story and podcast system-specific guidance layered after the shared persona.
- `storygen-user-story.txt`: user prompt template for story requests.
- `storygen-user-podcast.txt`: user prompt template for podcast requests.
- `aware-system.txt`: aware-mode system guidance.
- `aware-kind-commentary.txt`: aware-mode commentary guidance.
- `aware-kind-observation.txt`: aware-mode observation guidance.
- `aware-kind-monologue.txt`: aware-mode monologue guidance.
- `aware-kind-story.txt`: aware-mode story guidance.
- `review-precheck-system.txt`: classification prompt for deciding whether a source is suitable for review.
- `review-precheck-user.txt`: user prompt template for review precheck.
- `review-system.txt`: main review-generation system guidance.
- `review-length-short.txt`: review size guidance for short sources.
- `review-length-medium.txt`: review size guidance for mid-sized sources.
- `review-length-long.txt`: review size guidance for long sources.
- `generation-memory-guidance.txt`: instruction block describing how prior outputs should be treated as memory.
- `generation-memory-section.txt`: wrapper template for the aggregated previous stories and commentary text.
- `canon-context-section.txt`: wrapper template for the compact canon summary passed into generation.
- `prompts-guide.md`: detailed prompt map and Mermaid diagram showing how the prompt layers connect.

## Template Variables

Some prompt files use `str.format` placeholders.

Story generation templates can use:

- `{topic}`
- `{topic_or_none}`
- `{has_topic}`
- `{max_words}`
- `{podcast_minutes}`

Generation memory section templates can use:

- `{entry_count}`
- `{total_entry_count}`
- `{truncated}`
- `{history_text}`

Canon context section templates can use:

- `{summary_path}`
- `{canon_summary}`

Review precheck templates can use:

- `{title}`
- `{word_count}`
- `{text}`

Keep placeholders intact unless you also update the code that fills them.
