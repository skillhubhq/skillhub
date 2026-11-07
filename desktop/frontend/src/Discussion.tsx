import { signInURL } from "./auth";
import { useState } from "react";
import { API, useUI } from "./ui";
import { dateLabel, t } from "./i18n";
import {
  Person,
  ResourceState,
  useMutationKey,
  useResource,
  type Comment,
  type Discussion as DiscussionData,
} from "./community-ui";
import { Icon } from "./Icon";
import { isWeb } from "./platform";
export function Discussion({
  source,
  id,
  onCountChange,
  summary,
}: {
  source: string;
  id: string;
  onCountChange?: () => void;
  summary: DiscussionData | null;
}) {
  const ui = useUI(),
    [epoch, setEpoch] = useState(0),
    key = useMutationKey();
  const comments = useResource(
    () => API.CommunityListComments(source, id, "", ""),
    [source, id, epoch],
  );
  const [body, setBody] = useState("");
  const refresh = () => {
    setEpoch((v) => v + 1);
    onCountChange?.();
  };
  return (
    <section className="discussion-section">
      <div className="discussion-heading">
        <h3>
          {t("Conversation", "讨论")}
          {summary && <span>{summary.comment_count}</span>}
        </h3>
        <p>
          {t(
            "Ask a question. Share what worked.",
            "提出问题，或分享你的使用经验。",
          )}
        </p>
      </div>
      {summary?.can_interact ? (
        <form
          className="comment-composer"
          onSubmit={(e) => {
            e.preventDefault();
            void ui.run(async () => {
              await API.CommunityCreateComment(
                source,
                id,
                "",
                body,
                await key([source, id, body, epoch]),
              );
              setBody("");
              refresh();
            });
          }}
        >
          <textarea
            aria-label={t("Add a comment", "发表评论")}
            placeholder={t("What would you like to share?", "想分享些什么？")}
            value={body}
            maxLength={4000}
            onChange={(e) => setBody(e.target.value)}
            required
          />
          <div>
            <span>
              {t(
                "Discussion belongs to this exact version.",
                "讨论仅属于当前精确版本。",
              )}
            </span>
            <button className="primary" disabled={ui.busy || !body.trim()}>
              {t("Post comment", "发表评论")}
            </button>
          </div>
        </form>
      ) : (
        summary && (
          <div className="signin-prompt">
            {isWeb ? (
              <a href={signInURL()}>
                {t("Sign in to join the conversation", "登录后参与讨论")}
              </a>
            ) : (
              <button onClick={() => ui.navigate("sources")}>
                {t("Sign in to join the conversation", "登录后参与讨论")}
              </button>
            )}
          </div>
        )
      )}
      <ResourceState {...comments} retry={comments.reload}>
        {comments.value?.items?.map((comment) => (
          <CommentThread
            key={comment.comment_id}
            source={source}
            comment={comment}
            canInteract={summary?.can_interact ?? false}
            changed={refresh}
            replied={() => {
              onCountChange?.();
            }}
          />
        ))}
        {!comments.value?.items?.length && (
          <div className="quiet-empty">
            <Icon name="message" />
            <p>
              {t(
                "Be the first to start a conversation.",
                "成为第一个参与讨论的人。",
              )}
            </p>
          </div>
        )}
        {comments.value?.next_page && (
          <button
            onClick={() =>
              void ui.run(async () => {
                const p = await API.CommunityListComments(
                  source,
                  id,
                  "",
                  comments.value!.next_page,
                );
                comments.setValue({
                  items: [...(comments.value?.items ?? []), ...(p.items ?? [])],
                  next_page: p.next_page,
                });
              })
            }
          >
            {t("More comments", "更多评论")}
          </button>
        )}
      </ResourceState>
    </section>
  );
}
function CommentThread({
  source,
  comment,
  canInteract,
  changed,
  replied,
}: {
  source: string;
  comment: Comment;
  canInteract: boolean;
  changed: () => void;
  replied: () => void;
}) {
  const ui = useUI(),
    key = useMutationKey();
  const [expanded, setExpanded] = useState(false),
    [replying, setReplying] = useState(false),
    [body, setBody] = useState(""),
    [epoch, setEpoch] = useState(0);
  const replies = useResource(
    async () =>
      expanded
        ? API.CommunityListComments(
            source,
            comment.release_id,
            comment.comment_id,
            "",
          )
        : { items: [], next_page: "" },
    [source, comment.comment_id, expanded, epoch],
  );
  function remove(value: Comment) {
    ui.confirm(
      t("Remove comment", "移除评论"),
      t(
        "The text will be removed. Replies will remain in the conversation.",
        "评论正文将被移除，回复会保留在讨论中。",
      ),
      async () => {
        await API.CommunityDeleteComment(
          source,
          value.comment_id,
          await key([source, value.comment_id, "delete"]),
        );
        if (value.parent_id) {
          setEpoch((v) => v + 1);
          replied();
        } else changed();
      },
    );
  }
  const row = (value: Comment) => (
    <div className={`comment-row ${value.parent_id ? "reply-row" : ""}`}>
      <div className="comment-meta">
        <Person source={source} person={value.author} />
        <time dateTime={value.created_at}>{dateLabel(value.created_at)}</time>
      </div>
      <p className={`comment-text ${value.deleted ? "deleted" : ""}`}>
        {value.deleted
          ? t("This comment was removed.", "这条评论已被移除。")
          : value.body}
      </p>
      <div className="comment-tools">
        {!value.parent_id && (
          <button onClick={() => setExpanded((v) => !v)}>
            {expanded
              ? t("Hide replies", "收起回复")
              : t("View replies", "查看回复")}
          </button>
        )}
        {!value.parent_id && canInteract && !value.deleted && (
          <button
            onClick={() => {
              setExpanded(true);
              setReplying((v) => !v);
            }}
          >
            {t("Reply", "回复")}
          </button>
        )}
        {value.can_delete && (
          <button onClick={() => remove(value)}>{t("Remove")}</button>
        )}
      </div>
    </div>
  );
  return (
    <article className="comment-thread">
      {row(comment)}
      {expanded && (
        <div className="replies">
          <ResourceState {...replies} retry={replies.reload}>
            {replies.value?.items?.map((value) => (
              <div key={value.comment_id}>{row(value)}</div>
            ))}
            {replies.value?.next_page && (
              <button
                onClick={() =>
                  void ui.run(async () => {
                    const p = await API.CommunityListComments(
                      source,
                      comment.release_id,
                      comment.comment_id,
                      replies.value!.next_page,
                    );
                    replies.setValue({
                      items: [
                        ...(replies.value?.items ?? []),
                        ...(p.items ?? []),
                      ],
                      next_page: p.next_page,
                    });
                  })
                }
              >
                {t("More replies", "更多回复")}
              </button>
            )}
          </ResourceState>
          {replying && (
            <form
              className="reply-composer"
              onSubmit={(e) => {
                e.preventDefault();
                void ui.run(async () => {
                  await API.CommunityCreateComment(
                    source,
                    comment.release_id,
                    comment.comment_id,
                    body,
                    await key([source, comment.comment_id, body, epoch]),
                  );
                  setBody("");
                  setReplying(false);
                  setEpoch((v) => v + 1);
                  replied();
                });
              }}
            >
              <textarea
                required
                maxLength={4000}
                aria-label={t("Write a reply", "撰写回复")}
                placeholder={t("Write a thoughtful reply…", "写下你的回复…")}
                value={body}
                onChange={(e) => setBody(e.target.value)}
              />
              <div className="actions">
                <button type="button" onClick={() => setReplying(false)}>
                  {t("Cancel")}
                </button>
                <button className="primary" disabled={ui.busy || !body.trim()}>
                  {t("Post reply", "发布回复")}
                </button>
              </div>
            </form>
          )}
        </div>
      )}
    </article>
  );
}
