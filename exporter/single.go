package exporter

import (
	"context"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/directconnect"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/health"
	healthTypes "github.com/aws/aws-sdk-go-v2/service/health/types"
)

func (m *Metrics) GetAccountEvents(healthType string) []HealthEvent {
	log.Info("Received request for scraping")
	ctx := context.TODO()
	weekAgo := time.Now().Add(time.Hour * -24 * 7)
	weeksAhead2 := time.Now().Add(time.Hour * 24 * 7 * 2)
	now := time.Now()
	oneHour := time.Now().Add(time.Hour * -1)
	filter := healthTypes.EventFilter{}
	switch healthType {
	case "scheduled":
		filter.StartTimes = []healthTypes.DateTimeRange{
			{
				From: &weekAgo,
				To:   &weeksAhead2,
			}}
		filter.EventTypeCategories = []healthTypes.EventTypeCategory{healthTypes.EventTypeCategoryScheduledChange}
	case "issue":
		filter.StartTimes = []healthTypes.DateTimeRange{
			{
				From: &oneHour,
				To:   &now,
			}}
		filter.EventTypeCategories = []healthTypes.EventTypeCategory{healthTypes.EventTypeCategoryIssue}
	}

	pag := health.NewDescribeEventsPaginator(
		m.health,
		&health.DescribeEventsInput{
			Filter: &filter,
		})

	updatedEvents := make([]HealthEvent, 0)

	for pag.HasMorePages() {
		events, err := pag.NextPage(ctx)
		if err != nil {
			panic(err.Error())
		}

		for _, event := range events.Events {
			enrichedEvent := m.EnrichEvents(ctx, event)
			updatedEvents = append(updatedEvents, enrichedEvent)
		}
	}

	m.lastScrape = now

	return updatedEvents
}

func (m *Metrics) EnrichEvents(ctx context.Context, event healthTypes.Event) HealthEvent {

	enrichedEvent := HealthEvent{Arn: event.Arn}

	m.getEventDetails(ctx, event, &enrichedEvent)

	m.getAffectedEntities(ctx, event, &enrichedEvent)

	m.getTags(ctx, event, &enrichedEvent)

	return enrichedEvent
}

func (m Metrics) getEventDetails(ctx context.Context, event healthTypes.Event, enrichedEvent *HealthEvent) {
	details, err := m.health.DescribeEventDetails(ctx, &health.DescribeEventDetailsInput{EventArns: []string{*event.Arn}})
	if err != nil {
		panic(err.Error())
	}

	enrichedEvent.Event = details.SuccessfulSet[0].Event
	enrichedEvent.EventDescription = details.SuccessfulSet[0].EventDescription
}

func (m Metrics) getAffectedEntities(ctx context.Context, event healthTypes.Event, enrichedEvent *HealthEvent) {
	pagResources := health.NewDescribeAffectedEntitiesPaginator(
		m.health,
		&health.DescribeAffectedEntitiesInput{Filter: &healthTypes.EntityFilter{EventArns: []string{*event.Arn}}})

	for pagResources.HasMorePages() {
		resources, err := pagResources.NextPage(ctx)
		if err != nil {
			panic(err.Error())
		}

		enrichedEvent.AffectedResources = append(enrichedEvent.AffectedResources, resources.Entities...)
	}

	enrichedEvent.EventScope = event.EventScopeCode
}

func (m Metrics) getTags(ctx context.Context, event healthTypes.Event, enrichedEvent *HealthEvent) {
	if *event.Service == "EC2" {
		if ec2cli, ok := m.ec2[*event.Region]; ok {
			resources := []string{}
			for _, v := range enrichedEvent.AffectedResources {
				resources = append(resources, *v.EntityValue)
			}
			//log.Info(fmt.Sprintf("resources to find tags for: %v", resources))
			output, err := ec2cli.DescribeTags(ctx, &ec2.DescribeTagsInput{
				Filters: []types.Filter{
					{
						Name:   aws.String("resource-id"),
						Values: resources,
					},
				},
			})
			if err != nil {
				log.Warn(fmt.Sprintf("Cannot fetch tags for resources %v, error: %s", resources, err))
			}
			enrichedEvent.Tags = map[string]string{}
			for _, t := range output.Tags {
				//	log.Info(fmt.Sprintf("key=%s value=%s", *t.Key, *t.Value))
				enrichedEvent.Tags[*t.Key] = *t.Value
			}

			//log.Info(fmt.Sprintf("tags %v", enrichedEvent.Tags))
		}
	}
	if *event.Service == "DIRECTCONNECT" {
		if dxcli, ok := m.dx[*event.Region]; ok {

			resources := []string{}
			for _, v := range enrichedEvent.AffectedResources {
				resources = append(resources, fmt.Sprintf("arn:aws:directconnect:%s:%s:%s/%s", *enrichedEvent.Event.Region, strings.Split(*v.EntityArn, ":")[4], strings.Split(*v.EntityValue, "-")[0], *v.EntityValue))
			}
			log.Info(fmt.Sprintf("resources to find tags for: %v", resources))
			output, err := dxcli.DescribeTags(ctx, &directconnect.DescribeTagsInput{
				ResourceArns: resources,
			})
			if err != nil {
				log.Warn(fmt.Sprintf("Cannot fetch tags for resources %v, error: %s", resources, err))
				return
			}
			enrichedEvent.Tags = map[string]string{}
			for _, rt := range output.ResourceTags {
				for _, t := range rt.Tags {
					//					log.Info(fmt.Sprintf("key=%s value=%s", *t.Key, *t.Value))
					enrichedEvent.Tags[*t.Key] = *t.Value
				}
			}

			//log.Info(fmt.Sprintf("tags %v", enrichedEvent.Tags))
		}
	}
}
