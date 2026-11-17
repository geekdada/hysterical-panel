package api

import (
	"github.com/getkin/kin-openapi/openapi3"
)

var announcementSchemaDefs = map[string]any{
	"AnnouncementSummary":         AnnouncementSummary{},
	"PinnedAnnouncementsResponse": PinnedAnnouncementsResponse{},
	"AnnouncementListResponse":    AnnouncementListResponse{},
	"Announcement":                Announcement{},
	"ManagedAnnouncement":         ManagedAnnouncement{},
	"ManagedAnnouncementDetail":   ManagedAnnouncementDetail{},
	"AnnouncementCreateRequest":   AnnouncementCreateRequest{},
	"AnnouncementUpdateRequest":   AnnouncementUpdateRequest{},
	"UploadedFile":                UploadedFile{},
}

var announcementRequiredFields = map[string][]string{
	"AnnouncementSummary":         {"id", "title", "created"},
	"PinnedAnnouncementsResponse": {"items", "has_published"},
	"AnnouncementListResponse":    {"items", "total", "page", "per_page"},
	"Announcement":                {"id", "title", "html", "created", "updated"},
	"ManagedAnnouncement":         {"id", "title", "pinned", "hidden", "created", "updated"},
	"ManagedAnnouncementDetail":   {"id", "title", "pinned", "hidden", "created", "updated", "html", "content"},
	"AnnouncementCreateRequest":   {"title", "html", "content"},
	"UploadedFile":                {"id", "name", "mime", "size", "path"},
}

// linkAnnouncementSchemas points the nested list items at the shared summary
// schema so the generated client reuses one type.
func linkAnnouncementSchemas(schemas openapi3.Schemas) {
	summaries := &openapi3.SchemaRef{Value: &openapi3.Schema{
		Type:  &openapi3.Types{"array"},
		Items: &openapi3.SchemaRef{Ref: "#/components/schemas/AnnouncementSummary"},
	}}
	for _, name := range []string{"PinnedAnnouncementsResponse", "AnnouncementListResponse"} {
		if s := schemas[name]; s != nil && s.Value != nil {
			s.Value.Properties["items"] = summaries
		}
	}
}

type announcementPathDeps struct {
	ref        func(string) *openapi3.SchemaRef
	withAuth   func(*openapi3.Operation)
	notFound   *openapi3.ResponseRef
	badRequest *openapi3.ResponseRef
}

func (d announcementPathDeps) op(id, summary, response, body string) *openapi3.Operation {
	return d.opWith(id, summary, d.ref(response), body)
}

func (d announcementPathDeps) opWith(id, summary string, response *openapi3.SchemaRef, body string) *openapi3.Operation {
	o := &openapi3.Operation{
		OperationID: id, Summary: summary, Tags: []string{"announcements"},
		Responses: openapi3.NewResponses(openapi3.WithStatus(200, &openapi3.ResponseRef{Value: &openapi3.Response{
			Description: ptr(summary),
			Content:     openapi3.NewContentWithJSONSchemaRef(response),
		}})),
	}
	if body != "" {
		o.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithRequired(true).WithJSONSchemaRef(d.ref(body))}
		o.Responses.Set("400", d.badRequest)
	}
	d.withAuth(o)
	return o
}

func (d announcementPathDeps) withNotFound(o *openapi3.Operation) *openapi3.Operation {
	o.Responses.Set("404", d.notFound)
	return o
}

func addAnnouncementPaths(t *openapi3.T, d announcementPathDeps) {
	idParams := openapi3.Parameters{{Value: &openapi3.Parameter{
		Name: "id", In: "path", Required: true,
		Schema: &openapi3.SchemaRef{Value: openapi3.NewStringSchema()},
	}}}
	pageParam := &openapi3.ParameterRef{Value: &openapi3.Parameter{
		Name: "page", In: "query", Description: "1-based page number. Each page holds 20 announcements.",
		Schema: &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()},
	}}

	list := d.op("listAnnouncements", "List visible announcements, newest first", "AnnouncementListResponse", "")
	list.Parameters = openapi3.Parameters{pageParam}
	t.Paths.Set("/api/panel/announcements", &openapi3.PathItem{
		Get:  list,
		Post: d.op("createAnnouncement", "Publish an announcement", "ManagedAnnouncementDetail", "AnnouncementCreateRequest"),
	})
	t.Paths.Set("/api/panel/announcements/pinned", &openapi3.PathItem{
		Get: d.op("listPinnedAnnouncements", "List pinned announcements for the user page", "PinnedAnnouncementsResponse", ""),
	})
	t.Paths.Set("/api/panel/announcements/{id}", &openapi3.PathItem{
		Parameters: idParams,
		Get:        d.withNotFound(d.op("getAnnouncement", "Read a visible announcement", "Announcement", "")),
		Patch:      d.withNotFound(d.op("updateAnnouncement", "Edit, pin or hide an announcement", "ManagedAnnouncementDetail", "AnnouncementUpdateRequest")),
		Delete:     d.withNotFound(d.op("deleteAnnouncement", "Delete an announcement permanently", "DeleteResponse", "")),
	})
	t.Paths.Set("/api/panel/announcements/manage", &openapi3.PathItem{
		Get: d.opWith("listManagedAnnouncements", "List every announcement, hidden ones included", &openapi3.SchemaRef{Value: &openapi3.Schema{
			Type:  &openapi3.Types{"array"},
			Items: d.ref("ManagedAnnouncement"),
		}}, ""),
	})
	t.Paths.Set("/api/panel/announcements/manage/{id}", &openapi3.PathItem{
		Parameters: idParams,
		Get:        d.withNotFound(d.op("getManagedAnnouncement", "Read an announcement with its editor document", "ManagedAnnouncementDetail", "")),
	})
	t.Paths.Set("/api/panel/files", &openapi3.PathItem{Post: d.uploadFileOp()})
}

func (d announcementPathDeps) uploadFileOp() *openapi3.Operation {
	o := d.op("uploadFile", "Upload one public file of up to 1 GiB", "UploadedFile", "")
	form := openapi3.NewObjectSchema().
		WithPropertyRef(uploadFormField, &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithFormat("binary")})
	form.Required = []string{uploadFormField}
	o.RequestBody = &openapi3.RequestBodyRef{Value: openapi3.NewRequestBody().WithRequired(true).
		WithFormDataSchema(form)}
	o.Responses.Set("400", d.badRequest)
	o.Responses.Set("413", &openapi3.ResponseRef{Value: &openapi3.Response{
		Description: ptr("File larger than 1 GiB"),
		Content:     openapi3.NewContentWithJSONSchemaRef(d.ref("ErrorResponse")),
	}})
	return o
}
