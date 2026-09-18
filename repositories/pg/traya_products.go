package pgrepo

import "context"

// ProductDesc joins product_sku_mapping with medicine_master.
type ProductDesc struct {
	ProductPrincipalID                                     string
	ProductPrice                                           float64
	ImageCDNPath, CartCDNImages, MobileImagePath           string
	SingleHalfImages, SingleImages                         string
	MedicineDisplayName, MedicineType, MedicineDescription string
	MedicineDetailedDisplayName, MedicineDosage            string
	MedicineDosageCode, MedicineInfo, MedicineComposition  string
}

// ProductsByPrincipalIDs mirrors ProductMapping.whereIn('product_principal_id', ids).fetchAll({withRelated:['medicine']}).
func (s *TrayaStore) ProductsByPrincipalIDs(ctx context.Context, ids []string) ([]ProductDesc, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := s.Pool.Query(ctx, `
SELECT p.product_principal_id::text, COALESCE(p.product_price,0)::float8,
       COALESCE(p.image_cdn_path,''), COALESCE(p.cart_cdn_images,''), COALESCE(p.mobile_image_path,''),
       COALESCE(p.single_half_images,''), COALESCE(p.single_images,''),
       COALESCE(m.display_name,''), COALESCE(m.type,''), COALESCE(m.description,''), COALESCE(m.detailed_display_name,''),
       COALESCE(m.dosage,''), COALESCE(m.dosage_code,''), COALESCE(m.info,''), COALESCE(m.composition,'')
FROM product_sku_mapping p
LEFT JOIN medicine_master m ON m.id = p.medicine_id
WHERE p.product_principal_id::text = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProductDesc
	for rows.Next() {
		var d ProductDesc
		if err := rows.Scan(&d.ProductPrincipalID, &d.ProductPrice, &d.ImageCDNPath, &d.CartCDNImages, &d.MobileImagePath, &d.SingleHalfImages, &d.SingleImages,
			&d.MedicineDisplayName, &d.MedicineType, &d.MedicineDescription, &d.MedicineDetailedDisplayName, &d.MedicineDosage, &d.MedicineDosageCode, &d.MedicineInfo, &d.MedicineComposition); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
