package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"greenpark/perencanaan/internal/domain"
	"greenpark/perencanaan/internal/service"
)

// Department Kanban board handlers (Trello/Cycle-style). All routes live on
// the authed mux except GET /api/board/attachments/{attId}, which is public
// and validates its own token (Authorization header OR ?token= query — same
// pattern as /api/ws) so <img>/<video> tags can load files directly.

/* ---- board view ---------------------------------------------------------- */

func (h *Handler) boardGet(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	writeJSON(w, http.StatusOK, h.svc.Board(user, bearerToken(r)))
}

/* ---- lists ---------------------------------------------------------------- */

func (h *Handler) boardAddList(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	l, err := h.svc.BoardAddList(user, in.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *Handler) boardPatchList(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Title *string `json:"title"`
		Index *int    `json:"index"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	l, err := h.svc.BoardUpdateList(user, r.PathValue("listId"), in.Title, in.Index)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *Handler) boardDeleteList(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteList(user, r.PathValue("listId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---- cards ---------------------------------------------------------------- */

func (h *Handler) boardAddCard(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		ListID string `json:"listId"`
		Title  string `json:"title"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.BoardAddCard(user, in.ListID, in.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handler) boardGetCard(w http.ResponseWriter, r *http.Request) {
	c, err := h.svc.BoardCardByID(r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) boardPatchCard(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in service.BoardCardPatch
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.BoardUpdateCard(user, r.PathValue("cardId"), in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) boardDeleteCard(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteCard(user, r.PathValue("cardId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---- members --------------------------------------------------------------- */

func (h *Handler) boardAddMember(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Username string `json:"username"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.BoardAddMember(user, bearerToken(r), r.PathValue("cardId"), in.Username)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) boardRemoveMember(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	c, err := h.svc.BoardRemoveMember(user, r.PathValue("cardId"), r.PathValue("username"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

/* ---- labels ---------------------------------------------------------------- */

func (h *Handler) boardAddLabel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	lb, err := h.svc.BoardAddLabel(in.Name, in.Color)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, lb)
}

func (h *Handler) boardPatchLabel(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	lb, err := h.svc.BoardUpdateLabel(user, r.PathValue("labelId"), in.Name, in.Color)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lb)
}

func (h *Handler) boardDeleteLabel(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteLabel(user, r.PathValue("labelId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) boardCardAddLabel(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		LabelID string `json:"labelId"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.BoardCardAddLabel(user, r.PathValue("cardId"), in.LabelID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Handler) boardCardRemoveLabel(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	c, err := h.svc.BoardCardRemoveLabel(user, r.PathValue("cardId"), r.PathValue("labelId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

/* ---- checklists ------------------------------------------------------------ */

func (h *Handler) boardAddChecklist(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Title string `json:"title"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	cl, err := h.svc.BoardAddChecklist(user, r.PathValue("cardId"), in.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cl)
}

func (h *Handler) boardPatchChecklist(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Title *string `json:"title"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	cl, err := h.svc.BoardUpdateChecklist(user, r.PathValue("cardId"), r.PathValue("clId"), in.Title)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cl)
}

func (h *Handler) boardDeleteChecklist(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteChecklist(user, r.PathValue("cardId"), r.PathValue("clId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) boardAddChecklistItem(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Text string `json:"text"`
		Due  string `json:"due"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	it, err := h.svc.BoardAddChecklistItem(user, r.PathValue("cardId"), r.PathValue("clId"), in.Text, in.Due)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, it)
}

func (h *Handler) boardPatchChecklistItem(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Text *string `json:"text"`
		Done *bool   `json:"done"`
		Due  *string `json:"due"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	it, err := h.svc.BoardUpdateChecklistItem(user, r.PathValue("cardId"), r.PathValue("clId"), r.PathValue("itemId"), in.Text, in.Done, in.Due)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (h *Handler) boardDeleteChecklistItem(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteChecklistItem(user, r.PathValue("cardId"), r.PathValue("clId"), r.PathValue("itemId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---- attachments ------------------------------------------------------------ */

// boardUploadLimitMsg is returned whenever the 1 GiB attachment cap is hit.
const boardUploadLimitMsg = "ukuran file melebihi batas maksimal 1GB"

// maxBytesExceeded reports whether err came from http.MaxBytesReader.
func maxBytesExceeded(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// boardUploadAttachment streams a multipart "file" part straight to a temp
// file in the upload dir (the body is NEVER buffered in memory — uploads can
// be up to 1 GiB), then registers it on the card.
func (h *Handler) boardUploadAttachment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	cardID := r.PathValue("cardId")

	// Reject unknown cards / non-contributors BEFORE consuming a huge body.
	if err := h.svc.BoardCanEditCard(user, cardID); err != nil {
		writeServiceError(w, err)
		return
	}

	// Cap the whole request at the attachment limit + multipart overhead.
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxBoardAttachmentBytes+16<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload harus multipart/form-data dengan field \"file\"")
		return
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if maxBytesExceeded(err) {
				writeError(w, http.StatusRequestEntityTooLarge, boardUploadLimitMsg)
				return
			}
			writeError(w, http.StatusBadRequest, "upload multipart tidak valid")
			return
		}
		if part.FormName() != "file" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}

		filename := filepath.Base(strings.TrimSpace(part.FileName()))
		if filename == "." || filename == string(filepath.Separator) {
			filename = ""
		}
		partMime := part.Header.Get("Content-Type")

		tmp, err := os.CreateTemp(h.svc.UploadDir(), ".upload-*.tmp")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "gagal membuat file sementara")
			return
		}
		tmpPath := tmp.Name()
		n, copyErr := io.Copy(tmp, io.LimitReader(part, service.MaxBoardAttachmentBytes+1))
		closeErr := tmp.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if maxBytesExceeded(copyErr) {
				writeError(w, http.StatusRequestEntityTooLarge, boardUploadLimitMsg)
				return
			}
			writeError(w, http.StatusBadRequest, "gagal membaca file upload")
			return
		}
		if n > service.MaxBoardAttachmentBytes {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusRequestEntityTooLarge, boardUploadLimitMsg)
			return
		}

		att, err := h.svc.BoardAddAttachment(user, cardID, filename, partMime, tmpPath, n)
		if err != nil {
			_ = os.Remove(tmpPath)
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, att)
		return
	}
	writeError(w, http.StatusBadRequest, "field \"file\" wajib diisi")
}

func (h *Handler) boardDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteAttachment(user, r.PathValue("cardId"), r.PathValue("attId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// boardServeAttachment serves an attachment file with Range support (video
// seeking). PUBLIC route: it validates its own token — Authorization header OR
// ?token= query — because <img>/<video>/<a download> cannot set headers.
// Any-division SSO tokens are accepted (the board is cross-division).
func (h *Handler) boardServeAttachment(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if _, ok := h.resolveUserAny(token); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	att, path, err := h.svc.BoardAttachment(r.PathValue("attId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file lampiran tidak ditemukan di penyimpanan")
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "gagal membaca file lampiran")
		return
	}
	if att.Mime != "" {
		w.Header().Set("Content-Type", att.Mime)
	}
	disp := "inline"
	if r.URL.Query().Get("download") == "1" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", contentDisposition(disp, att.Name))
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// contentDisposition builds an RFC 6266 header value with both the ASCII
// fallback filename and the RFC 5987 UTF-8 filename*.
func contentDisposition(disp, filename string) string {
	fallback := make([]byte, 0, len(filename))
	for _, c := range []byte(filename) {
		if c < 0x20 || c >= 0x7f || c == '"' || c == '\\' {
			fallback = append(fallback, '_')
			continue
		}
		fallback = append(fallback, c)
	}
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disp, fallback, encodeRFC5987(filename))
}

// encodeRFC5987 percent-encodes a UTF-8 string per RFC 5987 attr-char rules.
func encodeRFC5987(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9'),
			strings.IndexByte("!#$&+-.^_`|~", c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

/* ---- comments ---------------------------------------------------------------- */

func (h *Handler) boardAddComment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	cm, err := h.svc.BoardAddComment(user, r.PathValue("cardId"), in.Text)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cm)
}

func (h *Handler) boardDeleteComment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.BoardDeleteComment(user, r.PathValue("cardId"), r.PathValue("commentId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// boardTaskAddComment appends a comment to a formal task's discussion thread.
func (h *Handler) boardTaskAddComment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Text string `json:"text"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	cm, err := h.svc.AddTaskComment(user, r.PathValue("projectId"), r.PathValue("taskId"), in.Text)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cm)
}

// boardTaskDeleteComment removes a task comment (author, PIC, or manager).
func (h *Handler) boardTaskDeleteComment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteTaskComment(user, r.PathValue("projectId"), r.PathValue("taskId"), r.PathValue("commentId")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

/* ---- Cek AI on a card attachment -------------------------------------------- */

// boardStartAICheck kicks off the async AI check of ONE card attachment (PDF or
// image) against a checklist skill. 409 when a run is already in flight for the
// card; 400 when the attachment type is unsupported.
func (h *Handler) boardStartAICheck(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		AttID string `json:"attId"`
		Skill string `json:"skill"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.BoardStartAICheck(user, r.PathValue("cardId"), in.AttID, in.Skill, bearerToken(r)); err != nil {
		if errors.Is(err, service.ErrBoardAIRunning) {
			writeError(w, http.StatusConflict, "pemeriksaan AI untuk kartu ini masih berjalan — tunggu sampai selesai")
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "running"})
}

// boardAICheckStatus returns the card's current Cek AI state (poll target).
func (h *Handler) boardAICheckStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.svc.BoardAICheckStatus(r.PathValue("cardId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

/* ---- formal-task proxy (task cards on the status columns) -------------------

The board GET injects each caller's formal tasks as read-model cards. These
endpoints let the board act on those tasks via the SAME service methods the
/api/projects/{id}/tasks/{taskId}/* routes use — only the path shape differs
({projectId}/{taskId}). They live on the board submux (resolveUserAny) so any
division's SSO user reaches them; the service still enforces PIC/manager perms.
The two GETs that serve PDF bytes are on the PUBLIC mux (self-validated token,
header OR ?token=) so <embed>/<iframe> can load them — see router.go. */

// boardTaskUpdate drags a task between status columns → UpdateTask.
func (h *Handler) boardTaskUpdate(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Status domain.TaskStatus `json:"status"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.UpdateTask(user, r.PathValue("projectId"), r.PathValue("taskId"), in.Status); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// boardTaskUploadDoc uploads a task's review PDF (moves it to Review) →
// UploadTaskDoc. Mirrors the projects-scoped uploadTaskDoc handler.
func (h *Handler) boardTaskUploadDoc(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read file")
		return
	}
	detail, err := h.svc.UploadTaskDoc(user, r.PathValue("projectId"), r.PathValue("taskId"), header.Filename, data)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// boardTaskDoc serves a task's review PDF inline. PUBLIC route: validates its own
// token (Authorization header OR ?token= query) so <embed>/<iframe> can load it,
// like boardServeAttachment. Any-division SSO tokens accepted.
func (h *Handler) boardTaskDoc(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if _, ok := h.resolveUserAny(token); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	data, name, err := h.svc.TaskDoc(r.PathValue("projectId"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", contentDisposition("inline", name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// boardTaskApprove approves a task's review (-> Selesai) → ApproveTask.
func (h *Handler) boardTaskApprove(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	detail, err := h.svc.ApproveTask(user, r.PathValue("projectId"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// boardTaskReject sends a task's review back to Proses → RejectTask. Body is
// optional: { "note": "…" } (also accepts "instruction" for parity).
func (h *Handler) boardTaskReject(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	var in struct {
		Note        string `json:"note"`
		Instruction string `json:"instruction"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	note := in.Note
	if note == "" {
		note = in.Instruction
	}
	detail, err := h.svc.RejectTask(user, r.PathValue("projectId"), r.PathValue("taskId"), note)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// boardTaskStartAI kicks off Deep Analisis AI on a task PDF → StartTaskAI. Body
// optional: { "skills": ["name", …], "attId": "<pdf task attachment id>" }.
// "attId" is optional — when set, that PDF task attachment is analysed instead of
// the review Doc (it MUST be a PDF, else 400). "skill" (singular) is still
// accepted for back-compat.
func (h *Handler) boardTaskStartAI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Skill  string   `json:"skill"`
		Skills []string `json:"skills"`
		AttID  string   `json:"attId"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	skills := in.Skills
	if len(skills) == 0 && strings.TrimSpace(in.Skill) != "" {
		skills = []string{in.Skill}
	}
	if err := h.svc.StartTaskAI(r.PathValue("projectId"), r.PathValue("taskId"), bearerToken(r), skills, in.AttID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "running"})
}

// boardTaskAIStatus returns the task's Deep Analisis state (progress + findings).
func (h *Handler) boardTaskAIStatus(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.TaskAIStatus(r.PathValue("projectId"), r.PathValue("taskId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// boardTaskAIPDF serves a task's annotated Deep Analisis result PDF inline.
// PUBLIC route with self-validated token (header OR ?token=), like boardTaskDoc.
func (h *Handler) boardTaskAIPDF(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if _, ok := h.resolveUserAny(token); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	data, name, ok := h.svc.TaskAIAnnotated(r.PathValue("projectId"), r.PathValue("taskId"))
	if !ok {
		writeServiceError(w, service.ErrNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", contentDisposition("inline", name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

/* ---- task attachments (multi-file, any type, on the board's task cards) ------

Mirror the FREE-card attachment flow exactly: streamed multipart upload (never
buffered — up to 1 GiB), disk bytes at <uploadDir>/<attId>, served with Range so
images/video/pdf preview and video seeking work. Editing is gated on the task's
PIC / a manager (canEditTask). The GET lives on the PUBLIC mux (self-validated
header OR ?token=) so <img>/<video>/<embed> can load files — see router.go. */

// boardTaskUploadAttachment streams a multipart "file" part straight to a temp
// file in the upload dir, then registers it on the task. ANY file type, 1 GiB
// cap → 413. Rejects non-editors BEFORE consuming the (huge) body.
func (h *Handler) boardTaskUploadAttachment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	projectID := r.PathValue("projectId")
	taskID := r.PathValue("taskId")

	// Reject unknown tasks / non-editors BEFORE consuming a huge body.
	if err := h.svc.CanEditTask(user, projectID, taskID); err != nil {
		writeServiceError(w, err)
		return
	}

	// Cap the whole request at the attachment limit + multipart overhead.
	r.Body = http.MaxBytesReader(w, r.Body, service.MaxBoardAttachmentBytes+16<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "upload harus multipart/form-data dengan field \"file\"")
		return
	}

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if maxBytesExceeded(err) {
				writeError(w, http.StatusRequestEntityTooLarge, boardUploadLimitMsg)
				return
			}
			writeError(w, http.StatusBadRequest, "upload multipart tidak valid")
			return
		}
		if part.FormName() != "file" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}

		filename := filepath.Base(strings.TrimSpace(part.FileName()))
		if filename == "." || filename == string(filepath.Separator) {
			filename = ""
		}
		partMime := part.Header.Get("Content-Type")

		tmp, err := os.CreateTemp(h.svc.UploadDir(), ".upload-*.tmp")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "gagal membuat file sementara")
			return
		}
		tmpPath := tmp.Name()
		n, copyErr := io.Copy(tmp, io.LimitReader(part, service.MaxBoardAttachmentBytes+1))
		closeErr := tmp.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if maxBytesExceeded(copyErr) {
				writeError(w, http.StatusRequestEntityTooLarge, boardUploadLimitMsg)
				return
			}
			writeError(w, http.StatusBadRequest, "gagal membaca file upload")
			return
		}
		if n > service.MaxBoardAttachmentBytes {
			_ = os.Remove(tmpPath)
			writeError(w, http.StatusRequestEntityTooLarge, boardUploadLimitMsg)
			return
		}

		att, err := h.svc.AddTaskAttachment(user, projectID, taskID, filename, partMime, tmpPath, n)
		if err != nil {
			_ = os.Remove(tmpPath)
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, att)
		return
	}
	writeError(w, http.StatusBadRequest, "field \"file\" wajib diisi")
}

// boardTaskDeleteAttachment removes a task attachment (editor only) → 204.
func (h *Handler) boardTaskDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	user, _ := userFromContext(r.Context())
	if err := h.svc.DeleteTaskAttachment(user, r.PathValue("projectId"), r.PathValue("taskId"), r.PathValue("attId")); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// boardTaskServeAttachment serves a task attachment file with Range support
// (video seeking). PUBLIC route: validates its own token (Authorization header
// OR ?token= query) so <img>/<video>/<a download> can load it. ?download=1
// forces an attachment disposition; inline otherwise. Mirrors
// boardServeAttachment exactly.
func (h *Handler) boardTaskServeAttachment(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if _, ok := h.resolveUserAny(token); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	att, path, err := h.svc.TaskAttachmentFile(r.PathValue("projectId"), r.PathValue("taskId"), r.PathValue("attId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file lampiran tidak ditemukan di penyimpanan")
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "gagal membaca file lampiran")
		return
	}
	if att.Mime != "" {
		w.Header().Set("Content-Type", att.Mime)
	}
	disp := "inline"
	if r.URL.Query().Get("download") == "1" {
		disp = "attachment"
	}
	w.Header().Set("Content-Disposition", contentDisposition(disp, att.Name))
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

/* ---- Deep Analisis skills picker -------------------------------------------- */

// boardSkills lists the available Deep Analisis skills for the picker as
// [{name,title}] (any authenticated board user).
func (h *Handler) boardSkills(w http.ResponseWriter, _ *http.Request) {
	list, err := h.svc.ListSkills()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	type skillOpt struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	}
	out := make([]skillOpt, 0, len(list))
	for _, sk := range list {
		out = append(out, skillOpt{Name: sk.Name, Title: sk.Title})
	}
	writeJSON(w, http.StatusOK, out)
}
