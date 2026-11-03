/** Limits from format/SPEC.md §5, §7 and §8. All readers enforce them. */
export const KIB = 1024;
export const MIB = 1024 * KIB;

/** §7: any `.md` file or `bearings.yaml`. */
export const MAX_FILE_BYTES = 1 * MIB;
/** §7. */
export const MAX_FRONTMATTER_BYTES = 16 * KIB;
/** §7. */
export const MAX_SECTIONS = 500;
/** §7. */
export const MAX_TOPICS_PER_SECTION = 2000;
/** §7. */
export const MAX_ENTRIES = 200_000;
/** §7: number of directories between the root and a file. */
export const MAX_DEPTH = 4;

/** §8 archives. */
export const MAX_ARCHIVE_ENTRIES = 210_000;
export const MAX_ARCHIVE_TOTAL_BYTES = 200 * MIB;
export const MAX_COMPRESSION_RATIO = 100;
/**
 * Not in the spec: the largest archive file the web app will read into memory.
 * A valid archive cannot be much larger than its total uncompressed size plus
 * per-entry ZIP overhead, so this rejects absurd inputs before reading them.
 */
export const MAX_ARCHIVE_FILE_BYTES = MAX_ARCHIVE_TOTAL_BYTES + 64 * MIB;

/** §5 field limits. */
export const MAX_NAME_CHARS = 80;
export const MAX_SECTION_TITLE_CHARS = 80;
export const MAX_SECTION_DESCRIPTION_CHARS = 2000;
export const MAX_TOPIC_TITLE_CHARS = 160;
export const MAX_TOPIC_NOTES_BYTES = 256 * KIB;
export const MAX_RESOURCES = 50;
export const MAX_URL_CHARS = 2048;
export const MAX_RESOURCE_TITLE_CHARS = 160;
export const MAX_ENTRY_BYTES = 64 * KIB;
export const MAX_TEMPLATE_TITLE_CHARS = 80;
export const MAX_TEMPLATE_DESCRIPTION_CHARS = 500;
export const MIN_ORDER = 0;
export const MAX_ORDER = 1_000_000;
