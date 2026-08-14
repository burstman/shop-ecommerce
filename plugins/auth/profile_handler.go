package auth

import (
	"fmt"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/layouts"

	"github.com/anthdm/superkit/kit"
	"github.com/gorilla/csrf"
	v "github.com/anthdm/superkit/validate"
)

var profileSchema = v.Schema{
	"firstName": v.Rules(v.Min(3), v.Max(50)),
	"lastName":  v.Rules(v.Min(3), v.Max(50)),
}

type ProfileFormValues struct {
	ID        uint   `form:"id"`
	FirstName string `form:"firstName"`
	LastName  string `form:"lastName"`
	Email     string
	Success   string
}

func HandleProfileShow(kit *kit.Kit) error {
	pluginAuth := kit.Auth().(models.AuthUser)

	var user User
	if err := db.Get().First(&user, pluginAuth.ID).Error; err != nil {
		return err
	}

	formValues := ProfileFormValues{
		ID:        user.ID,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Email:     user.Email,
	}

	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	csrfToken := csrf.Token(kit.Request)

	return kit.Render(layouts.App(pluginAuth, config.FromContext(kit.Request.Context()), categories, cart.Total, ProfileShow(formValues), csrfToken))
}

func HandleProfileUpdate(kit *kit.Kit) error {
	var values ProfileFormValues
	errors, ok := v.Request(kit.Request, &values, profileSchema)
	if !ok {
		return kit.Render(ProfileForm(values, errors))
	}

	pluginAuth := kit.Auth().(models.AuthUser)
	if pluginAuth.ID != values.ID {
		return fmt.Errorf("unauthorized request for profile %d", values.ID)
	}
	err := db.Get().Model(&User{}).
		Where("id = ?", pluginAuth.ID).
		Updates(&User{
			FirstName: values.FirstName,
			LastName:  values.LastName,
		}).Error
	if err != nil {
		return err
	}

	values.Success = "Profile successfully updated!"
	values.Email = pluginAuth.Email

	return kit.Render(ProfileForm(values, v.Errors{}))
}
