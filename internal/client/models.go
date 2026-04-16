package client

// models.go contains all Go structs mirroring the Kaizen API request/response types.

// ---------------------------------------------------------------------------
// Common models
// ---------------------------------------------------------------------------

// User represents the authenticated user profile.
type User struct {
	ID                    string         `json:"id"`
	Email                 string         `json:"email"`
	UserStatus            string         `json:"userStatus"`
	Roles                 []string       `json:"roles"`
	Profile               *UserProfile   `json:"profile"`
	DefaultOrganizationID *string        `json:"defaultOrganizationId"`
	Organizations         []Organization `json:"organizations"`
}

// UserProfile holds the user's name details.
type UserProfile struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// Organization represents a user's organization membership.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// PaginatedResponse wraps paginated API responses.
type PaginatedResponse[T any] struct {
	Content       []T `json:"content"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
	Size          int `json:"size"`
	Number        int `json:"number"`
}

// APIResponse wraps the standard CustomResponse envelope.
type APIResponse[T any] struct {
	Data T `json:"data"`
}

// APIError represents an error response from the API.
type APIError struct {
	Status  int      `json:"status"`
	Message string   `json:"message"`
	Errors  []string `json:"errors,omitempty"`
}

func (e *APIError) Error() string {
	return e.Message
}

// ---------------------------------------------------------------------------
// Board models
// ---------------------------------------------------------------------------

// Board represents a Kaizen board.
type Board struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Prefix      string       `json:"prefix"`
	Color       *string      `json:"color"`
	Icon        *string      `json:"icon"`
	ChildBoards []ChildBoard `json:"childBoards"`
	CreatedAt   string    `json:"createdAt"`
	UpdatedAt   string    `json:"updatedAt"`
}

// ChildBoard is a lightweight board reference used in parent board listings.
type ChildBoard struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// BoardCreateRequest is the payload for creating a board.
type BoardCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Prefix      string `json:"prefix"`
}

// BoardUpdateRequest is the payload for updating a board.
type BoardUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Prefix      *string `json:"prefix,omitempty"`
	Color       *string `json:"color,omitempty"`
	Icon        *string `json:"icon,omitempty"`
}

// ---------------------------------------------------------------------------
// Ticket models
// ---------------------------------------------------------------------------

// TicketPersonRef represents a person reference on a ticket (assignee, reviewer, createdBy).
type TicketPersonRef struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Email     string `json:"email"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// TicketProjectRef is a lightweight project reference on a ticket.
type TicketProjectRef struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// TicketBoardRef is a lightweight board reference on a ticket.
type TicketBoardRef struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// TicketSprintRef is a lightweight sprint reference on a ticket.
type TicketSprintRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TicketBacklogRef is a lightweight backlog reference on a ticket.
type TicketBacklogRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Ticket represents a Kaizen ticket in list views.
type Ticket struct {
	ID             string            `json:"id"`
	Key            string            `json:"key"`
	Title          string            `json:"title"`
	Status         string            `json:"status"`
	Priority       string            `json:"priority"`
	Type           string            `json:"type"`
	Percentage     int               `json:"percentage"`
	Weight         *int              `json:"weight"`
	DueDate        *string           `json:"dueDate"`
	CreatedAt      string         `json:"createdAt"`
	CreatedBy      TicketPersonRef   `json:"createdBy"`
	Assignees      []TicketPersonRef `json:"assignees"`
	Reviewers      []TicketPersonRef `json:"reviewers"`
	Project        *TicketProjectRef `json:"project"`
	Labels         []Label           `json:"labels"`
	ParentTicketID *string           `json:"parentTicketId"`
	SubTickets     []Ticket          `json:"subTickets"`
	Board          *TicketBoardRef   `json:"board"`
	Sprint         *TicketSprintRef  `json:"sprint"`
	Backlog        *TicketBacklogRef `json:"backlog"`
}

// TicketDetail extends Ticket with full detail fields.
type TicketDetail struct {
	Ticket
	Description    *string `json:"description"`
	RCA            *string `json:"rca"`
	LessonsLearned *string `json:"lessonsLearned"`
	MissionRank    *string `json:"missionRank"`
	DisplayOrder   int     `json:"displayOrder"`
}

// TicketCreateRequest is the payload for creating a ticket.
type TicketCreateRequest struct {
	Title          string   `json:"title"`
	Type           string   `json:"type"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	Description    *string  `json:"description,omitempty"`
	SprintID       *string  `json:"sprintId,omitempty"`
	BacklogID      *string  `json:"backlogId,omitempty"`
	ProjectID      *string  `json:"projectId,omitempty"`
	AssigneeIDs    []string `json:"assigneeIds,omitempty"`
	ReviewerIDs    []string `json:"reviewerIds,omitempty"`
	LabelIDs       []string `json:"labelIds,omitempty"`
	ParentTicketID *string  `json:"parentTicketId,omitempty"`
	Weight         *int     `json:"weight,omitempty"`
	DueDate        *string  `json:"dueDate,omitempty"`
}

// TicketUpdateRequest is the payload for updating a ticket.
type TicketUpdateRequest struct {
	Title          *string  `json:"title,omitempty"`
	Description    *string  `json:"description,omitempty"`
	Type           *string  `json:"type,omitempty"`
	Status         *string  `json:"status,omitempty"`
	Priority       *string  `json:"priority,omitempty"`
	SprintID       *string  `json:"sprintId,omitempty"`
	BacklogID      *string  `json:"backlogId,omitempty"`
	ProjectID      *string  `json:"projectId,omitempty"`
	ParentTicketID *string  `json:"parentTicketId,omitempty"`
	Percentage     *int     `json:"percentage,omitempty"`
	Weight         *int     `json:"weight,omitempty"`
	MissionRank    *string  `json:"missionRank,omitempty"`
	RCA            *string  `json:"rca,omitempty"`
	LessonsLearned *string  `json:"lessonsLearned,omitempty"`
	DueDate        *string  `json:"dueDate,omitempty"`
	AssigneeIDs    []string `json:"assigneeIds,omitempty"`
	ReviewerIDs    []string `json:"reviewerIds,omitempty"`
	LabelIDs       []string `json:"labelIds,omitempty"`
}

// TicketMoveRequest is the payload for moving a ticket to another board/sprint/backlog.
type TicketMoveRequest struct {
	TargetBoardID   *string `json:"targetBoardId,omitempty"`
	TargetSprintID  *string `json:"targetSprintId,omitempty"`
	TargetBacklogID *string `json:"targetBacklogId,omitempty"`
}

// BulkMoveRequest is the payload for moving multiple tickets at once.
type BulkMoveRequest struct {
	TicketIDs       []string `json:"ticketIds"`
	TargetSprintID  *string  `json:"targetSprintId,omitempty"`
	TargetBacklogID *string  `json:"targetBacklogId,omitempty"`
}

// TicketOrderRequest is the payload for reordering a ticket.
type TicketOrderRequest struct {
	Order     int     `json:"order"`
	SprintID  *string `json:"sprintId,omitempty"`
	BacklogID *string `json:"backlogId,omitempty"`
}

// ---------------------------------------------------------------------------
// Sprint models
// ---------------------------------------------------------------------------

// Sprint represents a Kaizen sprint.
type Sprint struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	StartDate   *string   `json:"startDate"`
	EndDate     *string   `json:"endDate"`
	BoardID     string    `json:"boardId"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// SprintCreateRequest is the payload for creating a sprint.
type SprintCreateRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	StartDate   *string `json:"startDate,omitempty"`
	EndDate     *string `json:"endDate,omitempty"`
}

// SprintUpdateRequest is the payload for updating a sprint.
type SprintUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
	StartDate   *string `json:"startDate,omitempty"`
	EndDate     *string `json:"endDate,omitempty"`
}

// SprintLinkRequest is the payload for linking tickets to a sprint.
type SprintLinkRequest struct {
	TicketIDs []string `json:"ticketIds"`
}

// ---------------------------------------------------------------------------
// Backlog models
// ---------------------------------------------------------------------------

// Backlog represents a board's backlog.
type Backlog struct {
	ID      string   `json:"id"`
	BoardID string   `json:"boardId"`
	Tickets []Ticket `json:"tickets"`
}

// ---------------------------------------------------------------------------
// Label models
// ---------------------------------------------------------------------------

// Label represents a Kaizen label.
type Label struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// LabelCreateRequest is the payload for creating a label.
type LabelCreateRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color,omitempty"`
}

// LabelUpdateRequest is the payload for updating a label.
type LabelUpdateRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// ---------------------------------------------------------------------------
// Project models
// ---------------------------------------------------------------------------

// Project represents a Kaizen project within a board.
type Project struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Color *string `json:"color"`
}

// ProjectCreateRequest is the payload for creating a project.
type ProjectCreateRequest struct {
	Name  string  `json:"name"`
	Color *string `json:"color,omitempty"`
}

// ProjectUpdateRequest is the payload for updating a project.
type ProjectUpdateRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// ---------------------------------------------------------------------------
// Member models
// ---------------------------------------------------------------------------

// BoardMember represents a member of a Kaizen board.
type BoardMember struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	Email     string    `json:"email"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Role      string    `json:"role"`
	CreatedAt string `json:"createdAt"`
}

// MemberAddRequest is the payload for adding a member to a board.
type MemberAddRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

// MemberUpdateRequest is the payload for updating a board member's role.
type MemberUpdateRequest struct {
	Role string `json:"role"`
}

// ---------------------------------------------------------------------------
// Comment models
// ---------------------------------------------------------------------------

// Comment represents a comment on a ticket.
type Comment struct {
	ID              string    `json:"id"`
	Content         string    `json:"content"`
	TicketID        string    `json:"ticketId"`
	AuthorID        string    `json:"authorId"`
	AuthorEmail     string    `json:"authorEmail"`
	AuthorFirstName string    `json:"authorFirstName"`
	AuthorLastName  string    `json:"authorLastName"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// CommentCreateRequest is the payload for creating a comment.
type CommentCreateRequest struct {
	Content string `json:"content"`
}

// CommentUpdateRequest is the payload for updating a comment.
type CommentUpdateRequest struct {
	Content string `json:"content"`
}
