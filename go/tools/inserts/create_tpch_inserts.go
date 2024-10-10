/*
Copyright 2024 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// main generates inserts for the TPC-H tables
// The size factor is specified by the user, and the inserts are generated such that they fit
// within the WHERE clauses of the test queries in t/tpch.test
func main() {
	// Specify the size factor
	var sizeFactor int
	fmt.Print("Enter size factor: ")
	_, _ = fmt.Scan(&sizeFactor)

	// Helper function to join values
	joinValues := func(values []string) string {
		return strings.Join(values, ",\n")
	}

	// Generate inserts for the region table
	fmt.Println("")
	var regionValues []string
	regionValues = append(regionValues, "(1, 'ASIA', 'Eastern Asia')")
	regionValues = append(regionValues, "(2, 'MIDDLE EAST', 'Rich cultural heritage')")
	regionValues = append(regionValues, "(3, 'EUROPE', 'Diverse cultures')")
	for i := 4; i <= sizeFactor; i++ {
		regionValues = append(regionValues, fmt.Sprintf("(%d, 'Region %d', 'Comment %d')", i, i, i))
	}
	fmt.Printf("INSERT INTO region (R_REGIONKEY, R_NAME, R_COMMENT) VALUES\n%s;\n\n", joinValues(regionValues))

	// Generate inserts for the nation table, include 'JAPAN', 'INDIA', 'EGYPT', and 'MOZAMBIQUE' for the test queries
	fmt.Println()
	var nationValues []string
	nationValues = append(nationValues, "(1, 'JAPAN', 1, 'Nation with advanced technology')")
	nationValues = append(nationValues, "(2, 'INDIA', 1, 'Nation with rich history')")
	nationValues = append(nationValues, "(3, 'MOZAMBIQUE', 2, 'Southern African nation')")
	nationValues = append(nationValues, "(4, 'EGYPT', 2, 'Ancient civilization')")
	for i := 5; i <= sizeFactor*12; i++ {
		regionKey := (i-1)%7 + 1
		nationValues = append(nationValues, fmt.Sprintf("(%d, 'Nation %d', %d, 'Nation Comment %d')", i, i, regionKey, i))
	}
	fmt.Printf("INSERT INTO nation (N_NATIONKEY, N_NAME, N_REGIONKEY, N_COMMENT) VALUES\n%s;\n\n", joinValues(nationValues))

	// Generate inserts for the supplier table
	fmt.Println()
	var supplierValues []string
	supplierValues = append(supplierValues, "(1, 'Supplier A', '123 Square', 1, '86-123-4567', 5000.00, 'High quality steel')")
	for i := 2; i <= sizeFactor*7; i++ {
		nationKey := (i-1)%12 + 1
		supplierValues = append(supplierValues, fmt.Sprintf("(%d, 'Supplier %d', 'Address %d', %d, 'Phone %d', %d, 'Supplier Comment %d')", i, i, i, nationKey, i, 5000+i*100, i))
	}
	fmt.Printf("INSERT INTO supplier (S_SUPPKEY, S_NAME, S_ADDRESS, S_NATIONKEY, S_PHONE, S_ACCTBAL, S_COMMENT) VALUES\n%s;\n\n", joinValues(supplierValues))

	// Generate inserts for the part table
	fmt.Println()
	var partValues []string
	partValues = append(partValues, "(1, 'Part dimension', 'MFGR A', 'Brand#52', 'SMALL PLATED COPPER', 3, 'SM BOX', 45.00, 'Part with special dimensions')")
	partValues = append(partValues, "(2, 'Large Brush', 'MFGR B', 'Brand#34', 'LARGE BRUSHED COPPER', 12, 'LG BOX', 30.00, 'Brush for industrial use')")
	for i := 3; i <= sizeFactor*5; i++ {
		partValues = append(partValues, fmt.Sprintf("(%d, 'Part %d', 'MFGR %d', 'Brand %d', 'Type %d', %d, 'Container %d', %.2f, 'Part Comment %d')", i, i, i, i, i, rand.Intn(100), i, float64(10+i*10), i))
	}
	fmt.Printf("INSERT INTO part (P_PARTKEY, P_NAME, P_MFGR, P_BRAND, P_TYPE, P_SIZE, P_CONTAINER, P_RETAILPRICE, P_COMMENT) VALUES\n%s;\n\n", joinValues(partValues))

	// Generate inserts for the partsupp table
	fmt.Println()
	var partsuppValues []string
	for i := 1; i <= sizeFactor*10; i++ {
		partKey := (i-1)%5 + 1
		suppKey := (i-1)%7 + 1
		partsuppValues = append(partsuppValues, fmt.Sprintf("(%d, %d, %d, %.2f, 'Partsupp Comment %d')", partKey, suppKey, rand.Intn(1000), float64(rand.Intn(2000))/100, i))
	}
	fmt.Printf("INSERT INTO partsupp (PS_PARTKEY, PS_SUPPKEY, PS_AVAILQTY, PS_SUPPLYCOST, PS_COMMENT) VALUES\n%s;\n\n", joinValues(partsuppValues))

	// Generate inserts for the customer table, include 'AUTOMOBILE' segment for test queries
	fmt.Println()
	var customerValues []string
	customerValues = append(customerValues, "(1, 'Customer A', '1234 Drive Lane', 1, '123-456-7890', 1000.00, 'AUTOMOBILE', 'Frequent automobile orders')")
	for i := 2; i <= sizeFactor*5; i++ {
		nationKey := (i-1)%12 + 1
		customerValues = append(customerValues, fmt.Sprintf("(%d, 'Customer %d', 'Address %d', %d, 'Phone %d', %.2f, 'Segment %d', 'Customer Comment %d')", i, i, i, nationKey, i, float64(rand.Intn(20000))/100, i%5, i))
	}
	fmt.Printf("INSERT INTO customer (C_CUSTKEY, C_NAME, C_ADDRESS, C_NATIONKEY, C_PHONE, C_ACCTBAL, C_MKTSEGMENT, C_COMMENT) VALUES\n%s;\n\n", joinValues(customerValues))

	// Generate inserts for the orders table
	fmt.Println()
	var orderValues []string
	orderValues = append(orderValues, "(1, 1, 'O', 15000.00, '1995-03-12', '1-URGENT', 'Clerk#0001', 1, 'Automobile related order')")
	for i := 2; i <= sizeFactor*5; i++ {
		custKey := (i-1)%5 + 1
		orderValues = append(orderValues, fmt.Sprintf("(%d, %d, 'O', %.2f, '1995-02-%02d', 'Priority %d', 'Clerk#%04d', %d, 'Order Comment %d')", i, custKey, float64(rand.Intn(50000)), rand.Intn(28)+1, rand.Intn(5)+1, i, rand.Intn(3)+1, i))
	}
	fmt.Printf("INSERT INTO orders (O_ORDERKEY, O_CUSTKEY, O_ORDERSTATUS, O_TOTALPRICE, O_ORDERDATE, O_ORDERPRIORITY, O_CLERK, O_SHIPPRIORITY, O_COMMENT) VALUES\n%s;\n\n", joinValues(orderValues))

	// Generate inserts for the lineitem table
	fmt.Println()
	var lineitemValues []string
	lineitemValues = append(lineitemValues, "(1, 1, 1, 1, 20, 5000.00, 0.05, 0.10, 'R', 'O', '1995-03-15', '1995-03-14', '1995-03-16', 'DELIVER IN PERSON', 'AIR', 'Handle with care')")
	lineitemValues = append(lineitemValues, "(2, 1, 2, 1, 30, 10000.00, 0.06, 0.05, 'N', 'F', '1995-03-17', '1995-03-16', '1995-03-18', 'NONE', 'RAIL', 'Bulk delivery')")
	for i := 3; i <= sizeFactor*10; i++ {
		orderKey := (i-1)%5 + 1
		partKey := (i-1)%5 + 1
		suppKey := (i-1)%7 + 1
		lineitemValues = append(lineitemValues, fmt.Sprintf("(%d, %d, %d, %d, %d, %.2f, %.2f, %.2f, 'N', 'O', '1995-03-%02d', '1995-03-%02d', '1995-03-%02d', 'DELIVER IN PERSON', 'TRUCK', 'Lineitem Comment %d')", orderKey, partKey, suppKey, i, rand.Intn(100), float64(rand.Intn(20000)), float64(rand.Intn(20))/100, float64(rand.Intn(10))/100, rand.Intn(28)+1, rand.Intn(28)+2, rand.Intn(28)+3, i))
	}
	fmt.Printf("INSERT INTO lineitem (L_ORDERKEY, L_PARTKEY, L_SUPPKEY, L_LINENUMBER, L_QUANTITY, L_EXTENDEDPRICE, L_DISCOUNT, L_TAX, L_RETURNFLAG, L_LINESTATUS, L_SHIPDATE, L_COMMITDATE, L_RECEIPTDATE, L_SHIPINSTRUCT, L_SHIPMODE, L_COMMENT) VALUES\n%s;\n\n", joinValues(lineitemValues))
}
