package helpers

import (
	"sort"

	"shopTemplate/app/db"
	"shopTemplate/app/models"

	"github.com/anthdm/superkit/kit"
)

// GetCategoryTree retrieves all categories from the database and organizes them into a hierarchical tree structure.
// Root categories (those without a parent) are placed at the top level, while child categories are nested under their parents.
// The resulting tree is sorted by the Position field for both root and all subcategories.
// Returns a slice of Category models representing the complete category hierarchy.
func GetCategoryTree() []models.Category {
	var allDbCategories []models.Category
	db.Get().Find(&allDbCategories)

	childrenMap := make(map[uint][]models.Category)
	var rootCategories []models.Category

	for _, c := range allDbCategories {
		if c.ParentID == nil {
			rootCategories = append(rootCategories, c)
		} else {
			childrenMap[*c.ParentID] = append(childrenMap[*c.ParentID], c)
		}
	}

	var buildTree func(cats []models.Category) []models.Category
	buildTree = func(cats []models.Category) []models.Category {
		for i := range cats {
			if subCategories, ok := childrenMap[cats[i].ID]; ok {
				cats[i].SubCategories = buildTree(subCategories)
			}
		}
		return cats
	}

	categories := buildTree(rootCategories)

	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Position < categories[j].Position
	})

	for i := range categories {
		SortSubCategories(&categories[i])
	}

	return categories
}

// SortSubCategories recursively sorts a category's subcategories by their Position field
// in ascending order, and then recursively sorts the subcategories of each subcategory.
// If the category has no subcategories, the function returns without performing any operations.
func SortSubCategories(category *models.Category) {
	if len(category.SubCategories) > 0 {
		sort.Slice(category.SubCategories, func(i, j int) bool {
			return category.SubCategories[i].Position < category.SubCategories[j].Position
		})
		for i := range category.SubCategories {
			SortSubCategories(&category.SubCategories[i])
		}
	}
}

// GetCart retrieves the shopping cart from the current session.
// If a cart exists in the session values, it returns the stored cart.
// Otherwise, it returns a newly initialized empty cart with no items.
func GetCart(kit *kit.Kit) *models.Cart {
	cart := &models.Cart{Items: make(map[uint]*models.CartItem)}
	sess := kit.GetSession("session")

	if val, ok := sess.Values["cart"]; ok {
		if c, ok := val.(*models.Cart); ok {
			cart = c
		}
	}

	return cart
}
